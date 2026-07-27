package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type cdpWebSocket struct {
	conn net.Conn
	rd   *bufio.Reader
}

func cdpDialWebSocket(rawURL string) (*cdpWebSocket, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
	}

	host := parsed.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}

	dialer := net.Dialer{Timeout: browserProbeTimeout}
	conn, err := dialer.Dial("tcp", host)
	if err != nil {
		return nil, err
	}

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}

	request := strings.Join([]string{
		fmt.Sprintf("GET %s HTTP/1.1", path),
		"Host: " + parsed.Host,
		"Upgrade: websocket",
		"Connection: Upgrade",
		"Sec-WebSocket-Key: " + key,
		"Sec-WebSocket-Version: 13",
		"",
		"",
	}, "\r\n")

	if _, err := conn.Write([]byte(request)); err != nil {
		conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(statusLine, "101") {
		conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(statusLine))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
	}

	return &cdpWebSocket{conn: conn, rd: reader}, nil
}

func (w *cdpWebSocket) Close() error {
	return w.conn.Close()
}

func (w *cdpWebSocket) SendJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.writeFrame(0x1, payload)
}

func (w *cdpWebSocket) ReadJSONUntil(deadline time.Time) (map[string]any, error) {
	for {
		if err := w.conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		op, payload, err := w.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case 0x1:
			var out map[string]any
			if err := json.Unmarshal(payload, &out); err != nil {
				continue
			}
			return out, nil
		case 0x8:
			return nil, io.EOF
		case 0x9:
			_ = w.writeFrame(0xA, payload)
		default:
		}
	}
}

func (w *cdpWebSocket) writeFrame(opcode byte, payload []byte) error {
	maskKey := make([]byte, 4)
	if _, err := rand.Read(maskKey); err != nil {
		return err
	}

	header := []byte{0x80 | (opcode & 0x0F)}
	payloadLen := len(payload)
	switch {
	case payloadLen <= 125:
		header = append(header, 0x80|byte(payloadLen))
	case payloadLen <= 65535:
		header = append(header, 0x80|126)
		ext := make([]byte, 2)
		binary.BigEndian.PutUint16(ext, uint16(payloadLen))
		header = append(header, ext...)
	default:
		header = append(header, 0x80|127)
		ext := make([]byte, 8)
		binary.BigEndian.PutUint64(ext, uint64(payloadLen))
		header = append(header, ext...)
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}

	packet := append(header, maskKey...)
	packet = append(packet, masked...)
	_, err := w.conn.Write(packet)
	return err
}

func (w *cdpWebSocket) readFrame() (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(w.rd, header); err != nil {
		return 0, nil, err
	}

	opcode := header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := int(header[1] & 0x7F)
	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(w.rd, ext); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(w.rd, ext); err != nil {
			return 0, nil, err
		}
		size := binary.BigEndian.Uint64(ext)
		if size > 8*1024*1024 {
			return 0, nil, errors.New("websocket frame too large")
		}
		length = int(size)
	}

	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(w.rd, maskKey); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(w.rd, payload); err != nil {
			return 0, nil, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return opcode, payload, nil
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		num, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return num
		}
	}
	return 0
}

func truncateText(value string, maxRunes int) string {
	clean := strings.TrimSpace(value)
	if maxRunes <= 0 {
		return clean
	}
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}
	return string(runes[:maxRunes]) + "..."
}
