package guacamole

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

// Client is a thin TCP client for guacd.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

// ClientInformation is the set of browser capabilities guacd requires before
// the final connect instruction. These instructions are part of the Guacamole
// handshake, not optional connection tuning.
type ClientInformation struct {
	Width          int
	Height         int
	DPI            int
	AudioMIMETypes []string
	VideoMIMETypes []string
	ImageMIMETypes []string
	Timezone       string
	Name           string
}

var guacamoleProtocolVersionPattern = regexp.MustCompile(`^VERSION_([0-9]+)_([0-9]+)_([0-9]+)$`)

var latestSupportedProtocolVersion = [3]int{1, 5, 0}

// Connect establishes a TCP connection to guacd.
func Connect(host string, port int) (*Client, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := securityruntime.DialOutboundTCPContext(context.Background(), host, port, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to guacd %s failed: %w", address, err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func (c *Client) SelectProtocol(protocol string) error {
	return c.send(EncodeInstruction("select", protocol))
}

func (c *Client) ReadInstruction() (string, []string, error) {
	return readInstruction(c.reader)
}

// SendHandshake sends the client capability preamble followed by connect.
// Guacd requires size/audio/video/image before connect even if some capability
// lists are empty. Optional timezone/name instructions are sent only when the
// negotiated protocol version supports them.
func (c *Client) SendHandshake(argNames []string, params map[string]string, info ClientInformation) error {
	if info.Width <= 0 || info.Height <= 0 || info.DPI <= 0 {
		return fmt.Errorf("invalid guacamole client dimensions")
	}
	if err := c.send(EncodeInstruction("size", strconv.Itoa(info.Width), strconv.Itoa(info.Height), strconv.Itoa(info.DPI))); err != nil {
		return err
	}
	if err := c.send(EncodeInstruction("audio", info.AudioMIMETypes...)); err != nil {
		return err
	}
	if err := c.send(EncodeInstruction("video", info.VideoMIMETypes...)); err != nil {
		return err
	}
	if err := c.send(EncodeInstruction("image", info.ImageMIMETypes...)); err != nil {
		return err
	}

	negotiated, hasVersion := negotiatedProtocolVersion(argNames)
	if hasVersion && versionAtLeast(negotiated, [3]int{1, 1, 0}) && info.Timezone != "" {
		if err := c.send(EncodeInstruction("timezone", info.Timezone)); err != nil {
			return err
		}
	}
	if hasVersion && versionAtLeast(negotiated, [3]int{1, 5, 0}) && info.Name != "" {
		if err := c.send(EncodeInstruction("name", info.Name)); err != nil {
			return err
		}
	}
	return c.SendConnect(argNames, params)
}

// SendConnect sends the connect values in the exact order advertised by the
// running guacd instance's preceding args instruction. The first advertised
// argument may be guacd's supported protocol version rather than a connection
// parameter; in that case the highest mutually supported version is returned.
// Guacd adds parameters across releases, so a hard-coded positional list would
// silently assign values to the wrong fields when client and daemon differ.
func (c *Client) SendConnect(argNames []string, params map[string]string) error {
	ordered := make([]string, 0, len(argNames))
	negotiated, hasVersion := negotiatedProtocolVersion(argNames)
	for i, name := range argNames {
		if i == 0 && hasVersion {
			ordered = append(ordered, formatProtocolVersion(negotiated))
			continue
		}
		ordered = append(ordered, params[name])
	}
	return c.send(EncodeInstruction("connect", ordered...))
}

func negotiatedProtocolVersion(argNames []string) ([3]int, bool) {
	if len(argNames) == 0 {
		return [3]int{}, false
	}
	matches := guacamoleProtocolVersionPattern.FindStringSubmatch(argNames[0])
	if len(matches) != 4 {
		return [3]int{}, false
	}
	serverVersion := [3]int{}
	for i := range serverVersion {
		parsed, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return [3]int{}, false
		}
		serverVersion[i] = parsed
	}
	if versionAtLeast(serverVersion, latestSupportedProtocolVersion) {
		return latestSupportedProtocolVersion, true
	}
	return serverVersion, true
}

func versionAtLeast(version, minimum [3]int) bool {
	for i := range version {
		if version[i] != minimum[i] {
			return version[i] > minimum[i]
		}
	}
	return true
}

func formatProtocolVersion(version [3]int) string {
	return fmt.Sprintf("VERSION_%d_%d_%d", version[0], version[1], version[2])
}

func (c *Client) send(instruction string) error {
	_, err := c.conn.Write([]byte(instruction))
	return err
}

func (c *Client) Write(data []byte) (int, error) {
	return c.conn.Write(data)
}

func (c *Client) Read(buf []byte) (int, error) {
	return c.reader.Read(buf)
}

func (c *Client) Reader() io.Reader {
	return c.reader
}

func (c *Client) SetDeadline(deadline time.Time) error {
	return c.conn.SetDeadline(deadline)
}

func (c *Client) Close() error {
	return c.conn.Close()
}
