// Package submgr places subscriptions with the subscription manager.
package submgr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coranlabs/OTAF/otaf-xapp-sdk/appmgr"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/e2"
	"github.com/coranlabs/OTAF/otaf-xapp-sdk/errs"
)

// Request asks a RAN to report. The descriptor is the one the xApp registered
// with, because the subscription manager returns its answer to the name the
// registration published and refuses a subscription it cannot answer.
type Request struct {
	Descriptor    appmgr.Descriptor
	Meid          string
	RANFunctionID int
	Subscription  *e2.Subscription
	InstanceID    int
	ActionID      int
}

// Response is what the subscription manager returns once it has accepted a
// request. Whether the RAN accepted it arrives later, over the callback.
type Response struct {
	SubscriptionID string
}

// Client talks to the subscription manager.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for a subscription manager reachable at addr, such as
// "service-ricplt-submgr-rest.ricplt:8088".
func New(addr string) *Client {
	base := addr
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	return &Client{BaseURL: strings.TrimSuffix(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func octets(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}

func (r Request) body() ([]byte, error) {
	if r.Subscription == nil {
		return nil, errs.New(errs.CategoryConfig, "SUBMGR_NO_SUBSCRIPTION", "nothing to subscribe to")
	}
	if r.Meid == "" {
		return nil, errs.New(errs.CategoryConfig, "SUBMGR_NO_MEID", "no RAN named")
	}
	if err := r.Descriptor.Validate(); err != nil {
		return nil, err
	}
	instance, action := r.InstanceID, r.ActionID
	if instance == 0 {
		instance = 1
	}
	if action == 0 {
		action = 1
	}
	host, _, _ := strings.Cut(r.Descriptor.RMREndpoint(), ":")
	return json.Marshal(map[string]any{
		"ClientEndpoint": map[string]any{
			"Host":     host,
			"HTTPPort": r.Descriptor.HTTPPort,
			"RMRPort":  r.Descriptor.RMRDataPort,
		},
		"Meid":          r.Meid,
		"RANFunctionID": r.RANFunctionID,
		"SubscriptionDetails": []map[string]any{{
			"XappEventInstanceID": instance,
			"EventTriggers":       octets(r.Subscription.EventTrigger),
			"ActionToBeSetupList": []map[string]any{{
				"ActionID":         action,
				"ActionType":       "report",
				"ActionDefinition": octets(r.Subscription.ActionDef),
			}},
		}},
	})
}

// Subscribe asks the RAN to start reporting. It returns once the subscription
// manager has taken the request, which is before the RAN has answered.
func (c *Client) Subscribe(ctx context.Context, req Request) (*Response, error) {
	body, err := req.body()
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodPost, "/ric/v1/subscriptions", body)
	if err != nil {
		return nil, err
	}
	var got struct {
		SubscriptionID string `json:"SubscriptionId"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return nil, errs.Wrap(err, errs.CategoryPlatform, "SUBMGR_BAD_RESPONSE", "subscription response")
	}
	return &Response{SubscriptionID: got.SubscriptionID}, nil
}

// Unsubscribe stops a subscription the subscription manager is holding.
func (c *Client) Unsubscribe(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/ric/v1/subscriptions/"+id, nil)
	return err
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, errs.Wrap(err, errs.CategoryPlatform, "SUBMGR_UNAVAILABLE", path)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	category, code := errs.CategoryPlatform, "SUBMGR_UNAVAILABLE"
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		category, code = errs.CategoryConfig, "SUBMGR_REJECTED"
	}
	return nil, errs.Newf(category, code, "%s: %s: %s", path, resp.Status,
		strings.TrimSpace(string(raw)))
}
