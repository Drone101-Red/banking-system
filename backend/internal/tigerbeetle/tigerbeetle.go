package tigerbeetle

import (
	"fmt"
	"strconv"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

type Client struct {
	client tb.Client
}

func New(address string, clusterID string) (*Client, error) {
	id, err := strconv.ParseUint(clusterID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cluster ID inválido: %w", err)
	}

	client, err := tb.NewClient(
		tb.ToUint128(id),
		[]string{address},
	)
	if err != nil {
		return nil, fmt.Errorf("crear cliente TigerBeetle: %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

func (c *Client) Ping() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("cliente TigerBeetle no inicializado")
	}

	if err := c.client.Nop(); err != nil {
		return fmt.Errorf("ping TigerBeetle: %w", err)
	}

	return nil
}

func (c *Client) Close() {
	if c == nil || c.client == nil {
		return
	}

	c.client.Close()
}
