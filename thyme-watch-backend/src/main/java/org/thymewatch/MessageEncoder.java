package org.thymewatch;

import org.jspecify.annotations.NonNull;
import org.jspecify.annotations.Nullable;

import com.google.gson.ExclusionStrategy;
import com.google.gson.FieldAttributes;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;

import jakarta.websocket.Encoder;
import jakarta.websocket.EndpointConfig;

public class MessageEncoder implements Encoder.Text<@NonNull Message> {

    private static Gson gson = new GsonBuilder().disableHtmlEscaping()
            .addSerializationExclusionStrategy(new ExclusionStrategy() {
                @Override
                public boolean shouldSkipField(@SuppressWarnings("null") FieldAttributes f) {
                    return f.getName() == "type";
                }

                @Override
                public boolean shouldSkipClass(@SuppressWarnings("null") Class<?> clazz) {
                    return false;
                }
            }).create();

    @Override
    public String encode(Message message) {
        return message.type.name() + " " + gson.toJson(message);
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