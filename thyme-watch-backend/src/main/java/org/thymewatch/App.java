package org.thymewatch;

import java.time.Instant;
import java.util.ArrayDeque;
import java.util.Collection;
import java.util.Collections;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.PriorityQueue;
import java.util.Queue;
import java.util.SortedMap;
import java.util.TreeMap;
import java.util.UUID;

import static java.util.Optional.empty;
import static java.util.Optional.of;
import static java.util.UUID.randomUUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicLong;
import java.util.stream.Collectors;

import org.jspecify.annotations.NonNull;
import org.jspecify.annotations.Nullable;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.client.HttpClientErrorException;
import org.springframework.web.context.request.async.WebAsyncTask;
import org.springframework.web.server.ResponseStatusException;

@RestController
@SpringBootApplication
public class App {
    private static final String template = "Hello, %s!";
    private static final AtomicLong counter = new AtomicLong();

    private static final Mutex<ListenersData> listenersData = new Mutex<>(
            new ListenersData(new TreeMap<>(), new PriorityQueue<>(
                    (a, b) -> a.caughtUpTo < b.caughtUpTo ? -1 : a.caughtUpTo == b.caughtUpTo ? 0 : 1)));
    private static final Mutex<MessagesData> messagesData = new Mutex<>(new MessagesData(new ArrayDeque<>(), 0));

    static record MessagesData(Queue<String> messages, long messageOffset) {
    };

    static record ListenersData(SortedMap<UUID, Listener> listeners, PriorityQueue<Listener> listenersByCaughtUp) {
    };

    static class Listener {
        UUID id;
        @NonNull
        CompletableFuture<String> request;
        @Nullable
        Instant disconnectedSince;
        long caughtUpTo;

        private Listener() {
        }
        
        private void connect(CompletableFuture<String> request) {
            disconnectedSince = null;
            this.request = request;
        }
        
        private void disonnect(String response) {
            request.complete(response);
            disonnect();
        }
        private void disonnect() {
            disconnectedSince = Instant.now();
            request = null;
        }

        public static Listener newDisconnected() {
            var ret = new Listener();
            ret.id = randomUUID();
            ret.request = null;
            ret.disconnectedSince = Instant.now();
            ret.caughtUpTo = -1;
            return ret;
        }
    }

    @RequestMapping("/")
    String home() {
        return "Hello World!";
    }

    @GetMapping("/hello")
    public String hello(@RequestParam(value = "name", defaultValue = "World") String name) {
        return String.format("Hello %s!", name);
    }

    @GetMapping("/recv")
    public WebAsyncTask<List<String>> receive(UUID id) {
        CompletableFuture<String> future;
        try (var lGuard = listenersData.lock(); var mGuard = messagesData.lock()) {
            var listener = lGuard.get().listeners.get(id);
            if (listener == null) {
                throw new ResponseStatusException(400, "unregistered", null);
            }
            if (listener.caughtUpTo < mGuard.get().messageOffset + mGuard.get().messages.size()) {
                // there may be messages the listener hasn't seen yet
                var firstIndex = Math.max(listener.caughtUpTo + 1 - mGuard.get().messageOffset, 0);
                List<String> list = mGuard.get().messages.stream().skip(firstIndex).collect(Collectors.toList());
                if (list.size() > 0) {
                    return new WebAsyncTask<>(() -> list);
                }
            }
            future = new CompletableFuture<String>();
            listener.request = future;
        }
        return new WebAsyncTask<List<String>>(-1, () -> {
            String ret;
            try {
                ret = future.get(60, TimeUnit.SECONDS);
            } catch (TimeoutException e) {
                try (var guard = listenersData.lock()) {
                    var listener = guard.get().listeners.get(id);
                    if (listener != null) {
                        listener.disconnectedSince = Instant.now();
                        listener.request = null;
                    }
                    throw new TimeoutException();
                }
            }
            return Collections.singletonList(ret);
        });
    }

    @PostMapping("/enlist")
    public UUID enlist() {
        var listener = Listener.newDisconnected();
        try (var guard = listenersData.lock()) {
            guard.get().listeners.put(listener.id, listener);
        }
        return listener.id;
    }

    @PostMapping("/post")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void post(@RequestParam String msg) {
        try (var guard = listenersData.lock()) {
            for (var listener : guard.get().listeners.values()) {
                if (listener.request != null) {
                    listener.request.complete(msg);
                }
            }
        }
        try (var guard = messagesData.lock()) {
            guard.get().messages.add(msg);
        }
    }

    @GetMapping("/greeting")
    public Greeting greeting(@RequestParam String name) {
        long currCounter = counter.incrementAndGet();
        return new Greeting(currCounter, template.formatted(name));
    }

    public static void main(String[] args) {
        SpringApplication.run(App.class, args);
    }
}
