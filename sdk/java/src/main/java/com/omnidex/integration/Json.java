package com.omnidex.integration;

import java.math.BigDecimal;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

final class Json {
    private Json() {}

    static Map<String, Object> parseObject(byte[] bytes) {
        try {
            String value = StandardCharsets.UTF_8.newDecoder()
                .onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT)
                .decode(ByteBuffer.wrap(bytes)).toString();
            return parseObject(value);
        } catch (CharacterCodingException error) {
            throw new IllegalStateException("Omnidex returned invalid UTF-8 JSON.", error);
        }
    }

    static Map<String, Object> parseObject(String source) {
        Object value = new Parser(source).parse();
        if (!(value instanceof Map<?, ?> map)) {
            throw new IllegalStateException("Omnidex response must be one JSON object.");
        }
        @SuppressWarnings("unchecked")
        Map<String, Object> result = (Map<String, Object>) map;
        return result;
    }

    static String encode(Object value) {
        StringBuilder output = new StringBuilder();
        write(output, value);
        return output.toString();
    }

    private static void write(StringBuilder output, Object value) {
        if (value == null) {
            output.append("null");
        } else if (value instanceof String text) {
            writeString(output, text);
        } else if (value instanceof Boolean || value instanceof Byte || value instanceof Short ||
            value instanceof Integer || value instanceof Long || value instanceof BigDecimal) {
            output.append(value);
        } else if (value instanceof Float number) {
            if (!Float.isFinite(number)) throw new IllegalArgumentException("JSON number must be finite.");
            output.append(number);
        } else if (value instanceof Double number) {
            if (!Double.isFinite(number)) throw new IllegalArgumentException("JSON number must be finite.");
            output.append(number);
        } else if (value instanceof Map<?, ?> map) {
            output.append('{');
            boolean separator = false;
            for (Map.Entry<?, ?> entry : map.entrySet()) {
                if (!(entry.getKey() instanceof String key)) {
                    throw new IllegalArgumentException("JSON object keys must be strings.");
                }
                if (separator) output.append(',');
                separator = true;
                writeString(output, key);
                output.append(':');
                write(output, entry.getValue());
            }
            output.append('}');
        } else if (value instanceof Iterable<?> iterable) {
            output.append('[');
            boolean separator = false;
            for (Object item : iterable) {
                if (separator) output.append(',');
                separator = true;
                write(output, item);
            }
            output.append(']');
        } else {
            throw new IllegalArgumentException("Value cannot be encoded as JSON: " + value.getClass().getName());
        }
    }

    private static void writeString(StringBuilder output, String value) {
        output.append('"');
        for (int index = 0; index < value.length(); index++) {
            char character = value.charAt(index);
            switch (character) {
                case '"' -> output.append("\\\"");
                case '\\' -> output.append("\\\\");
                case '\b' -> output.append("\\b");
                case '\f' -> output.append("\\f");
                case '\n' -> output.append("\\n");
                case '\r' -> output.append("\\r");
                case '\t' -> output.append("\\t");
                default -> {
                    if (character < 0x20) {
                        output.append(String.format("\\u%04x", (int) character));
                    } else if (Character.isHighSurrogate(character)) {
                        if (index + 1 >= value.length() || !Character.isLowSurrogate(value.charAt(index + 1))) {
                            throw new IllegalArgumentException("JSON string contains an unpaired surrogate.");
                        }
                        output.append(character).append(value.charAt(++index));
                    } else if (Character.isLowSurrogate(character)) {
                        throw new IllegalArgumentException("JSON string contains an unpaired surrogate.");
                    } else {
                        output.append(character);
                    }
                }
            }
        }
        output.append('"');
    }

    private static final class Parser {
        private final String source;
        private int index;

        private Parser(String source) {
            if (source == null) throw new IllegalStateException("Omnidex returned invalid JSON.");
            this.source = source;
        }

        private Object parse() {
            skipWhitespace();
            Object value = value();
            skipWhitespace();
            if (index != source.length()) fail();
            return value;
        }

        private Object value() {
            if (index >= source.length()) return fail();
            return switch (source.charAt(index)) {
                case '{' -> object();
                case '[' -> array();
                case '"' -> string();
                case 't' -> literal("true", true);
                case 'f' -> literal("false", false);
                case 'n' -> literal("null", null);
                default -> number();
            };
        }

        private Map<String, Object> object() {
            index++;
            LinkedHashMap<String, Object> result = new LinkedHashMap<>();
            skipWhitespace();
            if (consume('}')) return result;
            while (true) {
                skipWhitespace();
                if (index >= source.length() || source.charAt(index) != '"') return fail();
                String key = string();
                skipWhitespace();
                if (!consume(':')) return fail();
                skipWhitespace();
                Object value = value();
                if (result.containsKey(key)) {
                    throw new IllegalStateException("Omnidex JSON contains a duplicate object key.");
                }
                result.put(key, value);
                skipWhitespace();
                if (consume('}')) return result;
                if (!consume(',')) return fail();
            }
        }

        private List<Object> array() {
            index++;
            ArrayList<Object> result = new ArrayList<>();
            skipWhitespace();
            if (consume(']')) return result;
            while (true) {
                skipWhitespace();
                result.add(value());
                skipWhitespace();
                if (consume(']')) return result;
                if (!consume(',')) return fail();
            }
        }

        private String string() {
            index++;
            StringBuilder result = new StringBuilder();
            while (index < source.length()) {
                char character = source.charAt(index++);
                if (character == '"') return result.toString();
                if (character < 0x20) return fail();
                if (character != '\\') {
                    appendScalar(result, character);
                    continue;
                }
                if (index >= source.length()) return fail();
                switch (source.charAt(index++)) {
                    case '"' -> result.append('"');
                    case '\\' -> result.append('\\');
                    case '/' -> result.append('/');
                    case 'b' -> result.append('\b');
                    case 'f' -> result.append('\f');
                    case 'n' -> result.append('\n');
                    case 'r' -> result.append('\r');
                    case 't' -> result.append('\t');
                    case 'u' -> appendEscapedScalar(result);
                    default -> { return fail(); }
                }
            }
            return fail();
        }

        private void appendScalar(StringBuilder result, char character) {
            if (Character.isHighSurrogate(character)) {
                if (index >= source.length() || !Character.isLowSurrogate(source.charAt(index))) fail();
                result.append(character).append(source.charAt(index++));
            } else if (Character.isLowSurrogate(character)) {
                fail();
            } else {
                result.append(character);
            }
        }

        private void appendEscapedScalar(StringBuilder result) {
            char character = unicode();
            if (Character.isHighSurrogate(character)) {
                if (index + 6 > source.length() || source.charAt(index++) != '\\' || source.charAt(index++) != 'u') fail();
                char low = unicode();
                if (!Character.isLowSurrogate(low)) fail();
                result.append(character).append(low);
            } else if (Character.isLowSurrogate(character)) {
                fail();
            } else {
                result.append(character);
            }
        }

        private char unicode() {
            if (index + 4 > source.length()) return fail();
            int value = 0;
            for (int end = index + 4; index < end; index++) {
                int digit = Character.digit(source.charAt(index), 16);
                if (digit < 0) return fail();
                value = value * 16 + digit;
            }
            return (char) value;
        }

        private Object literal(String expected, Object value) {
            if (!source.startsWith(expected, index)) return fail();
            index += expected.length();
            return value;
        }

        private Number number() {
            int start = index;
            if (consume('-') && index >= source.length()) return fail();
            if (consume('0')) {
                if (index < source.length() && Character.isDigit(source.charAt(index))) return fail();
            } else {
                digits();
            }
            boolean decimal = false;
            if (consume('.')) { decimal = true; digits(); }
            if (consume('e') || consume('E')) {
                decimal = true;
                if (!consume('+')) consume('-');
                digits();
            }
            String token = source.substring(start, index);
            try {
                return decimal ? new BigDecimal(token) : Long.parseLong(token);
            } catch (NumberFormatException error) {
                return fail();
            }
        }

        private void digits() {
            int start = index;
            while (index < source.length() && Character.isDigit(source.charAt(index))) index++;
            if (start == index) fail();
        }

        private boolean consume(char expected) {
            if (index < source.length() && source.charAt(index) == expected) { index++; return true; }
            return false;
        }

        private void skipWhitespace() {
            while (index < source.length() && " \t\r\n".indexOf(source.charAt(index)) >= 0) index++;
        }

        private <T> T fail() {
            throw new IllegalStateException("Omnidex returned invalid JSON at character " + index + '.');
        }
    }
}
