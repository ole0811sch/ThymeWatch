package org.thymewatch;

public class Utils {
    static <T> T fail(String message) {
        throw new AssertionError(message);
    }
    static <T> T fail() {
        return fail("Failed");
    }
}
