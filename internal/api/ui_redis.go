package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type uiRedisClient struct {
	addr     string
	password string
	db       int
	tls      bool
	timeout  time.Duration
}

func newUIRedisClient(raw string) (*uiRedisClient, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return nil, fmt.Errorf("unsupported redis scheme %q", parsed.Scheme)
	}
	db := 0
	if path := strings.Trim(parsed.Path, "/"); path != "" {
		if db, err = strconv.Atoi(path); err != nil {
			return nil, fmt.Errorf("invalid redis db %q", path)
		}
	}
	password, _ := parsed.User.Password()
	return &uiRedisClient{
		addr:     parsed.Host,
		password: password,
		db:       db,
		tls:      parsed.Scheme == "rediss",
		timeout:  2 * time.Second,
	}, nil
}

func (c *uiRedisClient) Get(ctx context.Context, key string) (string, bool, error) {
	reply, err := c.command(ctx, "GET", key)
	if errors.Is(err, errRedisNil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return reply, true, nil
}

func (c *uiRedisClient) SetEX(ctx context.Context, key, value string, ttl time.Duration) error {
	seconds := int64(ttl.Seconds())
	if seconds < 60 {
		return fmt.Errorf("redis UI session TTL must be at least one minute")
	}
	_, err := c.command(ctx, "SETEX", key, strconv.FormatInt(seconds, 10), value)
	return err
}

func (c *uiRedisClient) Ping(ctx context.Context) error {
	reply, err := c.command(ctx, "PING")
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(reply), "PONG") {
		return fmt.Errorf("unexpected redis PING reply %q", reply)
	}
	return nil
}

var errRedisNil = errors.New("redis nil")

func (c *uiRedisClient) command(ctx context.Context, args ...string) (string, error) {
	if c == nil {
		return "", errRedisNil
	}
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return "", err
	}
	if c.tls {
		conn = tls.Client(conn, &tls.Config{ServerName: strings.Split(c.addr, ":")[0]})
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	reader := bufio.NewReader(conn)
	if c.password != "" {
		if err := writeRedisCommand(conn, "AUTH", c.password); err != nil {
			return "", err
		}
		if _, err := readRedisReply(reader); err != nil {
			return "", err
		}
	}
	if c.db > 0 {
		if err := writeRedisCommand(conn, "SELECT", strconv.Itoa(c.db)); err != nil {
			return "", err
		}
		if _, err := readRedisReply(reader); err != nil {
			return "", err
		}
	}
	if err := writeRedisCommand(conn, args...); err != nil {
		return "", err
	}
	return readRedisReply(reader)
}

func writeRedisCommand(w io.Writer, args ...string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return nil
}

func readRedisReply(r *bufio.Reader) (string, error) {
	kind, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch kind {
	case '+':
		return line, nil
	case '-':
		return "", errors.New(line)
	case ':':
		return line, nil
	case '$':
		n, err := strconv.Atoi(line)
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", errRedisNil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	default:
		return "", fmt.Errorf("unknown redis reply %q", string(kind))
	}
}
