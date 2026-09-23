package org.thymewatch;

import jakarta.websocket.server.ServerEndpoint;

import java.io.IOException;
import java.math.BigInteger;
import java.time.Instant;
import java.util.Scanner;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.function.BiConsumer;

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
        Server server = new Server("localhost", 8090, "/", null, App.class);

        try {
            server.start();
            System.out.println("WebSocket server started at ws://localhost:8080/ws/websocket");

            System.out.println("Press Enter to stop the server...");
            try (var scanner = new Scanner(System.in)) {
                while (true) {
                    var line = scanner.nextLine();
                    if (line.length() == 0) {
                        break;
                    }
                    int mins;
                    try {
                        mins = Integer.parseUnsignedInt(line);
                    } catch (NumberFormatException e) {
                        System.out.println("Invalid number");
                        continue;
                    }
                    var instant = Instant.now().plusSeconds(mins * 60);
                    var millis = BigInteger.valueOf(instant.getEpochSecond()).multiply(BigInteger.valueOf(1000))
                            .add(BigInteger.valueOf(instant.getNano() / 1000000));
                    var didSetCounter = new boolean[] { false };
                    synchronized (sessionDataMap) {
                        for (var dataMap : sessionDataMap.values()) {
                            var session = dataMap.lock().get().session;
                            sendMessage(session, new SetAlarmMessage(new PartialMessage(-1, session), millis),
                                    (_session, response) -> {
                                        if (!didSetCounter[0]) {
                                            System.out.println(response);
                                            didSetCounter[0] = true;
                                        }
                                    });
                        }
                    }
                }
            }
        } catch (Exception e) {
            e.printStackTrace();
        } finally {
            server.stop();
            System.out.println("WebSocket server stopped.");
        }
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