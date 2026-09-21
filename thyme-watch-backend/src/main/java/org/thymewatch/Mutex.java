package org.thymewatch;

import java.util.concurrent.locks.ReentrantLock;

import org.jspecify.annotations.Nullable;

public class Mutex<T> {
    public static class MutexGuard<T> implements AutoCloseable {
        private @Nullable Mutex<T> m;

        private MutexGuard(Mutex<T> m) {
            this.m = m;
        }

        public T get() {
            return this.m.val;
        }

        public void set(T val) {
            this.m.val = val;
        }

        @Override
        public void close() {
            this.m.unlock();
            this.m = null;
        }
    }
    
    private T val;
    private ReentrantLock lock;

    public Mutex(T val) {
        this.val = val;
        this.lock = new ReentrantLock();
    }
    
    public MutexGuard<T> lock() {
        lock.lock();
        try {
            return new MutexGuard<T>(this);
        } catch (Exception e) {
            lock.unlock();
            throw e;
        }
    }

    private void unlock() {
        lock.unlock();
    }
}
