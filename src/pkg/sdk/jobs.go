package sdk

import (
	"encoding/json"
	"fmt"
	"time"
)

type Job struct {
	ID              uint      `json:"id"`
	PipelineName    string    `json:"pipeline_name"`
	PipelineVersion string    `json:"pipeline_version"`
	Status          string    `json:"status"`
	DispatchID      string    `json:"dispatch_id"`
	AllocID         string    `json:"alloc_id"`
	CreatedAt       time.Time `json:"created_at"`
}

func (c *Client) ListJobs(page, pageSize int) ([]Job, error) {
	path := fmt.Sprintf("/user/jobs?page=%d&page_size=%d", page, pageSize)
	resp, err := c.get(path, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Code != 2000 {
		return nil, fmt.Errorf("list jobs: %s", result.Msg)
	}

	jobsRaw, _ := json.Marshal(result.Data["jobs"])
	var jobs []Job
	if err := json.Unmarshal(jobsRaw, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

type SubmitJobRequest struct {
	PipelineName    string `json:"pipeline_name"`
	PipelineVersion string `json:"pipeline_version"`
	PipelineParams  any    `json:"pipeline_params"`
}

func (c *Client) SubmitJob(req SubmitJobRequest) error {
	resp, err := c.post("/job/add", req, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != 2000 {
		return fmt.Errorf("submit job: %s", result.Msg)
	}
	return nil
}
