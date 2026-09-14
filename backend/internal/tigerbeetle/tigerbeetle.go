package tigerbeetle

import (
	"fmt"
	"strconv"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"
)

const (
	maxConnectionAttempts = 5
	connectionRetryDelay   = 2 * time.Second
)

type Client struct {
	client tb.Client
}

func New(address string, clusterID string) (*Client, error) {
	id, err := strconv.ParseUint(clusterID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cluster ID inválido: %w", err)
	}

	var connectedClient *Client

	err = retry(maxConnectionAttempts, connectionRetryDelay, func() error {
		client, err := tb.NewClient(
			tb.ToUint128(id),
			[]string{address},
		)
		if err != nil {
			return fmt.Errorf("crear cliente TigerBeetle: %w", err)
		}

		candidate := &Client{
			client: client,
		}

		if err := candidate.Ping(); err != nil {
			candidate.Close()
			return err
		}

		connectedClient = candidate
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf(
			"TigerBeetle no disponible después de %d intentos: %w",
			maxConnectionAttempts,
			err,
		)
	}

	return connectedClient, nil
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
