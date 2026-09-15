package types

import "fmt"

type PipelineAddDto struct {
	Name        string `json:"name" binding:"required,nonblank"`
	Version     string `json:"version" binding:"required,nonblank"`
	Repository  string `json:"repository" binding:"required,nonblank"`
	Author      string `json:"author"`
	Description string `json:"description"`
}

func (p *PipelineAddDto) JobId() string {
	return fmt.Sprintf("%s-%s", p.Name, p.Version)
}

type PipelineDeleteDto struct {
	Name    string `json:"name" binding:"required,nonblank"`
	Version string `json:"version" binding:"required,nonblank"`
}

func (p *PipelineDeleteDto) JobId() string {
	return fmt.Sprintf("%s-%s", p.Name, p.Version)
}

type PipelineListDto struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Repository  string `json:"repository"`
	Author      string `json:"author"`
	Description string `json:"description"`
}
