package org.thymewatch;

import jakarta.websocket.server.ServerEndpoint;

import java.io.IOException;
import java.math.BigInteger;
import java.text.SimpleDateFormat;
import java.time.Instant;
import java.util.Date;
import java.util.Locale;
import java.util.Scanner;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.concurrent.TimeUnit;
import java.util.function.BiConsumer;
import java.util.function.Consumer;

import org.glassfish.grizzly.http.Method;
import org.glassfish.grizzly.http.server.HttpHandler;
import org.glassfish.grizzly.http.server.HttpServer;
import org.glassfish.grizzly.http.server.Request;
import org.glassfish.grizzly.http.server.Response;
import org.glassfish.grizzly.http.server.TimeoutHandler;
import org.glassfish.grizzly.http.util.HttpStatus;
import org.glassfish.tyrus.server.Server;
import org.jspecify.annotations.Nullable;

import jakarta.websocket.EncodeException;
import jakarta.websocket.EndpointConfig;
import jakarta.websocket.OnClose;
import jakarta.websocket.server.ServerEndpointConfig;
import jakarta.websocket.OnError;
import jakarta.websocket.OnMessage;
import jakarta.websocket.OnOpen;
import jakarta.websocket.Session;
import static org.thymewatch.Message.*;

@ServerEndpoint(value = "/", decoders = MessageDecoder.class, encoders = MessageEncoder.class, configurator = App.Configurator.class)
public class App {
    public final static ConcurrentMap<String, Mutex<SessionData>> sessionDataMap = new ConcurrentHashMap<>();
    public final static ConcurrentMap<String, ConcurrentMap<Integer, BiConsumer<Session, Message>>> sessionCallbacksMap = new ConcurrentHashMap<>();
    public final static String ORIGIN = "http://localhost:5173";
    public final static int HTTP_PORT = 8091;
    public final static int WS_PORT = 8090;

    public static class SessionData {
        public int nextMessageId = 0;
        public Session session;

        public SessionData(Session session) {
            this.session = session;
        }
    }

    public static class Configurator extends ServerEndpointConfig.Configurator {
        @Override
        public boolean checkOrigin(@Nullable String originHeaderValue) {
            System.out.println("Origin: " + originHeaderValue);
            return originHeaderValue == null || originHeaderValue.equals(ORIGIN);
        }
    }

    public static void main(String[] args) {
        Server server = new Server("localhost", WS_PORT, "/", null, App.class);
        try {
            server.start();

            HttpServer httpServer = HttpServer.createSimpleServer(null, "127.0.0.1", HTTP_PORT);
            httpServer.getServerConfiguration().addHttpHandler(
                    new HttpHandler() {
                        public void service(@SuppressWarnings("null") Request request,
                                @SuppressWarnings("null") Response response) throws Exception {
                            System.out.println(request.getHttpHandlerPath());
                            if (request.getHttpHandlerPath().equals("/alarm")) {
                                if (!request.getMethod().equals(Method.POST)) {
                                    response.setStatus(HttpStatus.METHOD_NOT_ALLOWED_405);
                                }
                                var secsSinceEpochStr = request.getParameter("expirytime");
                                double secsSinceEpoch;
                                try {
                                    secsSinceEpoch = Double.parseDouble(secsSinceEpochStr);
                                } catch (NumberFormatException e) {
                                    response.setStatus(HttpStatus.BAD_REQUEST_400);
                                    response.getWriter().write("expirytime");
                                    return;
                                }
                                System.out.println("1");
                                response.suspend(5, TimeUnit.SECONDS, null, response2 -> {
                                    System.out.println("timeout");
                                    response2.setStatus(HttpStatus.GATEWAY_TIMEOUT_504);
                                    response2.resume();
                                    return true;
                                });
                                var couldRequest = setAlarm(secsSinceEpoch,
                                        (session, message) -> {
                                            System.out.println("Response");
                                            System.out.println(message);
                                            if (message instanceof OkResponseMessage) {
                                                response.setStatus(HttpStatus.OK_200);
                                                response.resume();
                                            } else {
                                                response.setStatus(HttpStatus.INTERNAL_SERVER_ERROR_500);
                                                response.resume();
                                            }
                                        });
                                if (!couldRequest) {
                                    System.out.println("unavailable");
                                    response.setStatus(HttpStatus.SERVICE_UNAVAILABLE_503);
                                    response.resume();
                                    return;
                                }
                            } else {
                                response.setStatus(HttpStatus.NOT_FOUND_404);
                            }
                            // final SimpleDateFormat format = new SimpleDateFormat("EEE, dd MMM yyyy
                            // HH:mm:ss zzz",
                            // Locale.US);
                            // final String date = format.format(new Date(System.currentTimeMillis()));
                            // response.setContentType("text/plain");
                            // response.setContentLength(date.length());
                            // response.getWriter().write(date);
                        }
                    });
            try {
                httpServer.start();
                System.out.println("Press any key to stop the server...");
                System.in.read();
            } catch (Exception e) {
                System.err.println(e);
            } finally {
                httpServer.shutdown();
            }
        } catch (Exception e) {
            e.printStackTrace();
        } finally {
            server.stop();
        }

    }

    public static boolean setAlarm(Double secsSinceEpoch, BiConsumer<Session, Message> callback) {
        var didSetCounter = new boolean[] { false };
        synchronized (sessionDataMap) {
            if (sessionDataMap.size() == 0) {
                return false;
            }
            for (var dataMap : sessionDataMap.values()) {
                System.out.println("Lock");
                try (var guard = dataMap.lock()) {
                    var session = guard.get().session;
                    System.out.println("Locked");
                    sendMessage(session, new SetAlarmMessage(new PartialMessage(-1, session), secsSinceEpoch),
                            (session2, response) -> {
                                System.out.println("responded");
                                if (!didSetCounter[0]) {
                                    System.out.println("first");
                                    callback.accept(session2, response);
                                    didSetCounter[0] = true;
                                }
                            });
                }
            }
        }
        return true;
    }

    public static void sendMessage(Session session, Message message, BiConsumer<Session, Message> callback) {
        synchronized (sessionCallbacksMap) {
            var map = sessionCallbacksMap.computeIfAbsent(session.getId(), key -> new ConcurrentHashMap<>());
            map.put(message.id, callback);
        }
        sendMessage(session, message);
    }

    public static void sendMessage(Session session, Message message) {
        session.getAsyncRemote()
                .sendObject(message);
    }

    @OnOpen
    public void onOpen(Session session, EndpointConfig config) throws IOException {
        sessionDataMap.put(session.getId(), new Mutex<SessionData>(new SessionData(session)));
    }

    @OnMessage
    public void onMessage(Session session, Message message) throws IOException, EncodeException {
        if (message.inResponseTo == -1) {
            message.handle(this, session);
            return;
        }
        var map = sessionCallbacksMap.get(session.getId());
        if (map != null) {
            var callback = map.get(message.inResponseTo);
            if (callback != null) {
                callback.accept(session, message);
                return;
            }
        }
        if (message.type == Message.Type.ERROR) {
            message.handle(this, session);
        }
    }

    @OnClose
    public void onClose(Session session) throws IOException {
        sessionDataMap.remove(session.getId());
        sessionCallbacksMap.remove(session.getId());
    }

    @OnError
    public void onError(Session session, Throwable throwable) {
        try {
            session.getBasicRemote()
                    .sendObject(new Message.ErrorMessage(throwable.getMessage(), new PartialMessage(-1, session)));
        } catch (IOException | EncodeException e) {
            e.printStackTrace();
        }
    }

    public void handleGetCapabilities(Session session, GetCapabilitiesMessage message)
            throws IOException, EncodeException {
        session.getBasicRemote().sendObject(new CapabilitiesMessage(new PartialMessage(message.id, session)));
    }

    public void handleError(Session session, ErrorMessage message)
            throws IOException, EncodeException {
        System.out.println("Client reported error: " + message.message);
    }
}