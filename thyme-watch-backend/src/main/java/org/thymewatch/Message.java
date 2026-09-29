package org.thymewatch;

import java.io.IOException;

import org.jspecify.annotations.Nullable;

import jakarta.websocket.EncodeException;
import jakarta.websocket.Session;

/**
 * Message
 */
public abstract class Message {
    public Type type;
    public final int inResponseTo;
    public final int id;

    public static enum Type {
        GET_CAPABILITIES(GetCapabilitiesMessage.class),
        CAPABILITIES(CapabilitiesMessage.class),
        ERROR(ErrorMessage.class),
        SET_ALARM(SetAlarmMessage.class),
        OK_RESPONSE(OkResponseMessage.class);

        public final Class<? extends Message> clazz;

        Type(Class<? extends Message> clazz) {
            this.clazz = clazz;
        }
    }

    public static record PartialMessage(int inResponseTo, Session session) {
        private PartialMessage() {
            this(Utils.fail(), Utils.fail());
        }
    }

    public static class SetAlarmMessage extends Message {
        Double time;
        @Nullable String name;

        /**
         * @param time Ms since epoch
         */
        public SetAlarmMessage(PartialMessage part, Double time, @Nullable String name) {
            super(Type.SET_ALARM, part);
            this.time = time;
            this.name = name;
        }
    }

    public static class OkResponseMessage extends Message {
        public OkResponseMessage(PartialMessage part) {
            super(Type.OK_RESPONSE, part);
        }
    }

    public static class GetCapabilitiesMessage extends Message {
        public GetCapabilitiesMessage(PartialMessage part) {
            super(Type.GET_CAPABILITIES, part);
        }

        @Override
        public void handle(App app, Session session) throws IOException, EncodeException {
            app.handleGetCapabilities(session, this);
        }
    }

    public static class CapabilitiesMessage extends Message {
        public CapabilitiesMessage(PartialMessage part) {
            super(Type.CAPABILITIES, part);
        }
    }

    public static class ErrorMessage extends Message {
        public final String message;

        public ErrorMessage(String message, PartialMessage part) {
            super(Type.ERROR, part);
            this.message = message;
        }
        
        @Override
        public void handle(App app, Session session) throws IOException, EncodeException {
            app.handleError(session, this);
        }
    }

    public Message(Type type, PartialMessage part) {
        this.type = type;
        this.inResponseTo = part.inResponseTo;
        var sessionDataLock = App.sessionDataMap.get(part.session.getId());
        if (sessionDataLock == null) {
            throw new SessionClosedException();
        }
        try (var data = sessionDataLock.lock()) {
            this.id = data.get().nextMessageId++;
        }
    }

    public void handle(App app, Session session) throws IOException, EncodeException {
        session.getBasicRemote()
                .sendObject(new ErrorMessage("Not implemented", new PartialMessage(inResponseTo, session)));
    }

    @Override
    public String toString() {
        return new MessageEncoder().encode(this);
    }
}
