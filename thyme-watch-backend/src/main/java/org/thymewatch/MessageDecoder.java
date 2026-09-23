package org.thymewatch;

import java.util.Objects;

import org.jspecify.annotations.NonNull;
import org.jspecify.annotations.Nullable;

import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import com.google.gson.JsonSyntaxException;

import jakarta.websocket.DecodeException;
import jakarta.websocket.Decoder;
import jakarta.websocket.EndpointConfig;

public class MessageDecoder implements Decoder.Text<@NonNull Message> {

    private static Gson gson = new GsonBuilder().disableHtmlEscaping().create();

    @Override
    public Message decode(@SuppressWarnings("null") String s) throws DecodeException {
        var parts = s.split(" ", 2);
        if (parts.length != 2) {
            throw new DecodeException(s, "Invalid message syntax");
        }
        Message.Type type;
        try {
            type = Objects.requireNonNull(Enum.valueOf(Message.Type.class, parts[0]));
        } catch (IllegalArgumentException e) {
            throw new DecodeException(s, "Invalid tag");
        }
        Message message;
        try {
            message = Objects.requireNonNull(gson.fromJson(parts[1], type.clazz));
        } catch (JsonSyntaxException e) {
            throw new DecodeException(s, "Invalid message json");
        }
        message.type = type;
        return message;
    }

    @Override
    public boolean willDecode(@Nullable String s) {
        return (s != null);
    }

    @Override
    public void init(@Nullable EndpointConfig endpointConfig) {
        // Custom initialization logic
    }

    @Override
    public void destroy() {
        // Close resources
    }
}
