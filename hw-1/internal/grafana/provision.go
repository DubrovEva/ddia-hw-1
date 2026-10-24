package grafana

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (c *Client) WaitHealthy(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		err := c.Health(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

func (c *Client) RotateServiceAccountToken(ctx context.Context, name, role string) (Token, error) {
	sa, found, err := c.FindServiceAccount(ctx, name)
	if err != nil {
		return Token{}, err
	}
	if !found {
		if sa, err = c.CreateServiceAccount(ctx, name, role); err != nil {
			return Token{}, err
		}
	}

	token, err := c.CreateToken(ctx, sa.ID, fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))
	if err != nil {
		return Token{}, err
	}
	if token.Key == "" {
		return Token{}, errors.New("grafana: created token has empty key")
	}

	tokens, err := c.ListTokens(ctx, sa.ID)
	if err != nil {
		return Token{}, err
	}
	for _, t := range tokens {
		if t.ID == token.ID {
			continue
		}
		if err := c.DeleteToken(ctx, sa.ID, t.ID); err != nil {
			return Token{}, err
		}
	}

	return token, nil
}
