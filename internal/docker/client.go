package docker

import (
	"context"
	"fmt"

	dockertypes "github.com/docker/docker/api/types"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"

	"docknudge/internal/events"
)

type EventSource interface {
	Ping(context.Context) error
	Stream(context.Context) (<-chan events.Event, <-chan error, error)
	Close() error
}

type Client struct {
	raw *dockerclient.Client
}

func New(host string) (*Client, error) {
	options := []dockerclient.Opt{dockerclient.WithAPIVersionNegotiation()}
	if host != "" {
		options = append(options, dockerclient.WithHost(host))
	}
	raw, err := dockerclient.NewClientWithOpts(options...)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Client{raw: raw}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.raw.Ping(ctx)
	if err != nil {
		return fmt.Errorf("ping docker: %w", err)
	}
	return nil
}

func (c *Client) Stream(ctx context.Context) (<-chan events.Event, <-chan error, error) {
	msgs, errs := c.raw.Events(ctx, dockertypes.EventsOptions{
		Filters: containerFilters(),
	})

	out := make(chan events.Event)
	outErr := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(outErr)
		for msgs != nil || errs != nil {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					msgs = nil
					continue
				}
				event, err := events.Normalize(msg)
				if err != nil {
					outErr <- err
					return
				}
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				if err != nil {
					outErr <- fmt.Errorf("docker stream error: %w", err)
					return
				}
			}
		}
	}()

	return out, outErr, nil
}

func (c *Client) Close() error {
	return c.raw.Close()
}

func containerFilters() dockerfilters.Args {
	args := dockerfilters.NewArgs()
	args.Add("type", "container")
	return args
}
