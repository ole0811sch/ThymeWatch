package org.thymewatch;

import java.util.Collection;
import java.util.HashMap;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.function.BiConsumer;
import static org.thymewatch.MessageMap.Value;

import jakarta.websocket.Session;

public class MessageMap implements Map<Class<? extends Message>, Value<Message>> {
    private final HashMap<Class<? extends Message>, Value<? extends Message>> map;

    record Value<T extends Message>(BiConsumer<Session, T> callback, int messageId) {
    }

    public MessageMap() {
        map = new HashMap<>();
    }

    @SuppressWarnings("unchecked")
    public <T extends Message> Optional<Value<T>> get(Class<? extends T> key) {
        var val = map.get(key);
        assert key.isInstance(val);
        return Optional.of((Value<T>) val);
    }

    public <T extends Message> Value<T> safePut(Class<T> key,
            Value<T> value) {
        return (Value<T>) map.put(key, value);
    }

    @Override
    public int size() {
        return map.size();
    }

    @Override
    public boolean isEmpty() {
        return map.isEmpty();
    }

    @Override
    public boolean containsKey(Object key) {
        return map.containsKey(key);
    }

    @Override
    public boolean containsValue(Object value) {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'containsValue'");
    }

    @Override
    public Value<Message> get(Object key) {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'get'");
    }

    @Override
    public Value<Message> put(Class<? extends Message> key, Value<Message> value) {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'put'");
    }

    @Override
    public Value<Message> remove(Object key) {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'remove'");
    }

    @Override
    public void putAll(Map<? extends Class<? extends Message>, ? extends Value<Message>> m) {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'putAll'");
    }

    @Override
    public void clear() {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'clear'");
    }

    @Override
    public Set<Class<? extends Message>> keySet() {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'keySet'");
    }

    @Override
    public Collection<Value<Message>> values() {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'values'");
    }

    @Override
    public Set<Entry<Class<? extends Message>, Value<Message>>> entrySet() {
        // TODO Auto-generated method stub
        throw new UnsupportedOperationException("Unimplemented method 'entrySet'");
    }
}
