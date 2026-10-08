package main

import (
	"context"
	"fmt"
	"net/url"
)

func (c client) agentCases(ctx context.Context, o options) (any, error) {
	path := fmt.Sprintf("/api/ai/agent/cases?limit=%d&after_id=%d", o.caseLimit, o.caseAfterID)
	if o.caseState != "" {
		path += "&state=" + url.QueryEscape(o.caseState)
	}
	response, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("get agent cases (requires updated backend): %w", err)
	}
	return response.Data, nil
}
