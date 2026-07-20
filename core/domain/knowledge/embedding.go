package knowledge

import (
	"fmt"
	"strings"
)

type EmbedInput struct {
	ID   string
	Text string
}

type EmbedRequest struct {
	EmbeddingProfileID string
	Inputs             []EmbedInput
}

type EmbedOutput struct {
	ID     string
	Vector []float32
}

type EmbedResult struct {
	EmbeddingProfileID string
	Dimensions         int
	Outputs            []EmbedOutput
}

func (request EmbedRequest) Validate() error {
	if strings.TrimSpace(request.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if len(request.Inputs) == 0 {
		return fmt.Errorf("embedding inputs are required")
	}
	for _, input := range request.Inputs {
		if strings.TrimSpace(input.ID) == "" {
			return fmt.Errorf("embedding input id is required")
		}
		if strings.TrimSpace(input.Text) == "" {
			return fmt.Errorf("embedding input text is required")
		}
	}
	return nil
}

func (result EmbedResult) Validate() error {
	if strings.TrimSpace(result.EmbeddingProfileID) == "" {
		return fmt.Errorf("embedding profile id is required")
	}
	if result.Dimensions < 1 {
		return fmt.Errorf("embedding dimensions must be positive")
	}
	for _, output := range result.Outputs {
		if strings.TrimSpace(output.ID) == "" {
			return fmt.Errorf("embedding output id is required")
		}
		if len(output.Vector) != result.Dimensions {
			return fmt.Errorf("embedding output vector must match dimensions")
		}
		if err := validateVector(output.Vector); err != nil {
			return err
		}
	}
	return nil
}
