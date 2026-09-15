package sdk

import (
	"encoding/json"
	"fmt"
)

type Pipeline struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Repository  string `json:"repository"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

func (c *Client) ListPipelines() ([]Pipeline, error) {
	resp, err := c.get("/pipeline/list_all", true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 2000 {
		return nil, fmt.Errorf("list pipelines: %s", result.Msg)
	}

	pipelinesRaw, _ := json.Marshal(result.Data["pipelines"])
	var pipelines []Pipeline
	if err := json.Unmarshal(pipelinesRaw, &pipelines); err != nil {
		return nil, err
	}
	return pipelines, nil
}
