package guacamole

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Client is a thin TCP client for guacd.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

// Connect establishes a TCP connection to guacd.
func Connect(host string, port int) (*Client, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
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
	raw, err := c.reader.ReadString(';')
	if err != nil {
		return "", nil, err
	}
	return ParseInstruction(raw)
}

// SendConnect sends a connect instruction with commonly used RDP parameters.
func (c *Client) SendConnect(params map[string]string) error {
	ordered := []string{
		params["hostname"],
		params["port"],
		params["username"],
		params["password"],
		params["domain"],
		params["security"],
		params["ignore-cert"],
		params["disable-auth"],
		params["width"],
		params["height"],
		params["dpi"],
		params["enable-audio"],
		params["enable-drive"],
		params["enable-printing"],
		params["drive-path"],
		params["create-drive-path"],
	}
	return c.send(EncodeInstruction("connect", ordered...))
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

func (c *Client) Close() error {
	return c.conn.Close()
}
