package cpi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cpars-innovation/cpicli/pkg/httpclnt"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultBatchSize for Partner Directory operations
	DefaultBatchSize = 90
)

// PartnerDirectory handles Partner Directory API operations
type PartnerDirectory struct {
	exe *httpclnt.HTTPExecuter
}

// NewPartnerDirectory creates a new Partner Directory API client
func NewPartnerDirectory(exe *httpclnt.HTTPExecuter) *PartnerDirectory {
	return &PartnerDirectory{
		exe: exe,
	}
}

// StringParameter represents a partner directory string parameter
type StringParameter struct {
	Pid              string `json:"Pid"`
	ID               string `json:"Id"`
	Value            string `json:"Value"`
	CreatedBy        string `json:"CreatedBy,omitempty"`
	LastModifiedBy   string `json:"LastModifiedBy,omitempty"`
	CreatedTime      string `json:"CreatedTime,omitempty"`
	LastModifiedTime string `json:"LastModifiedTime,omitempty"`
}

// BinaryParameter represents a partner directory binary parameter
type BinaryParameter struct {
	Pid              string `json:"Pid"`
	ID               string `json:"Id"`
	Value            string `json:"Value"` // Base64 encoded
	ContentType      string `json:"ContentType"`
	CreatedBy        string `json:"CreatedBy,omitempty"`
	LastModifiedBy   string `json:"LastModifiedBy,omitempty"`
	CreatedTime      string `json:"CreatedTime,omitempty"`
	LastModifiedTime string `json:"LastModifiedTime,omitempty"`
}

// BatchResult represents the results of a batch operation
type BatchResult struct {
	Created   []string `json:"created,omitempty"`
	Updated   []string `json:"updated,omitempty"`
	Unchanged []string `json:"unchanged,omitempty"`
	Deleted   []string `json:"deleted,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// GetStringParameters retrieves all string parameters from partner directory
func (pd *PartnerDirectory) GetStringParameters(selectFields string) ([]StringParameter, error) {
	basePath := "/api/v1/StringParameters"
	separator := "?"
	if selectFields != "" {
		basePath += "?$select=" + url.QueryEscape(selectFields)
		separator = "&"
	}

	allParameters := []StringParameter{}
	skip := 0
	batchSize := 1000
	totalCount := -1

	for {
		path := fmt.Sprintf("%s%s$inlinecount=allpages&$top=%d&$skip=%d", basePath, separator, batchSize, skip)
		log.Debug().Msgf("Getting string parameters from %s", path)

		resp, err := pd.exe.ExecGetRequest(path, map[string]string{
			"Accept": "application/json",
		})
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("get string parameters failed with response code = %d", resp.StatusCode)
		}

		body, err := pd.exe.ReadRespBody(resp)
		if err != nil {
			return nil, err
		}

		var result struct {
			D struct {
				Results []StringParameter `json:"results"`
				Count   string            `json:"__count"`
			} `json:"d"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Parse total count on first iteration
		if totalCount == -1 && result.D.Count != "" {
			fmt.Sscanf(result.D.Count, "%d", &totalCount)
			log.Info().Msgf("Total string parameters available: %d", totalCount)
		}

		batchCount := len(result.D.Results)
		allParameters = append(allParameters, result.D.Results...)

		if totalCount > 0 {
			log.Debug().Msgf("Retrieved %d string parameters in this batch (progress: %d/%d)", batchCount, len(allParameters), totalCount)
		} else {
			log.Debug().Msgf("Retrieved %d string parameters in this batch (total so far: %d)", batchCount, len(allParameters))
		}

		// If we got fewer results than batch size, we've reached the end
		if batchCount < batchSize {
			break
		}

		skip += batchSize
	}

	log.Info().Msgf("Retrieved %d total string parameters", len(allParameters))
	return allParameters, nil
}

// GetBinaryParameters retrieves all binary parameters from partner directory
func (pd *PartnerDirectory) GetBinaryParameters(selectFields string) ([]BinaryParameter, error) {
	basePath := "/api/v1/BinaryParameters"
	separator := "?"
	if selectFields != "" {
		basePath += "?$select=" + url.QueryEscape(selectFields)
		separator = "&"
	}

	allParameters := []BinaryParameter{}
	skip := 0
	batchSize := 30 // Binary parameters API has a lower limit than string parameters
	totalCount := -1

	for {
		path := fmt.Sprintf("%s%s$inlinecount=allpages&$top=%d&$skip=%d", basePath, separator, batchSize, skip)
		log.Debug().Msgf("Getting binary parameters from %s", path)

		resp, err := pd.exe.ExecGetRequest(path, map[string]string{
			"Accept": "application/json",
		})
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("get binary parameters failed with response code = %d", resp.StatusCode)
		}

		body, err := pd.exe.ReadRespBody(resp)
		if err != nil {
			return nil, err
		}

		var result struct {
			D struct {
				Results []BinaryParameter `json:"results"`
				Count   string            `json:"__count"`
			} `json:"d"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Parse total count on first iteration
		if totalCount == -1 && result.D.Count != "" {
			fmt.Sscanf(result.D.Count, "%d", &totalCount)
			log.Info().Msgf("Total binary parameters available: %d", totalCount)
		}

		batchCount := len(result.D.Results)
		allParameters = append(allParameters, result.D.Results...)

		if totalCount > 0 {
			log.Debug().Msgf("Retrieved %d binary parameters in this batch (progress: %d/%d)", batchCount, len(allParameters), totalCount)
		} else {
			log.Debug().Msgf("Retrieved %d binary parameters in this batch (total so far: %d)", batchCount, len(allParameters))
		}

		// If we got fewer results than batch size, we've reached the end
		if batchCount < batchSize {
			break
		}

		skip += batchSize
	}

	log.Info().Msgf("Retrieved %d total binary parameters", len(allParameters))
	return allParameters, nil
}

// GetStringParameter retrieves a single string parameter
func (pd *PartnerDirectory) GetStringParameter(pid, id string) (*StringParameter, error) {
	path := fmt.Sprintf("/api/v1/StringParameters(Pid='%s',Id='%s')",
		url.QueryEscape(pid),
		url.QueryEscape(id))

	log.Debug().Msgf("Getting string parameter %s/%s", pid, id)

	resp, err := pd.exe.ExecGetRequest(path, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get string parameter failed with response code = %d", resp.StatusCode)
	}

	body, err := pd.exe.ReadRespBody(resp)
	if err != nil {
		return nil, err
	}

	var result struct {
		D StringParameter `json:"d"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result.D, nil
}

// GetBinaryParameter retrieves a single binary parameter
func (pd *PartnerDirectory) GetBinaryParameter(pid, id string) (*BinaryParameter, error) {
	path := fmt.Sprintf("/api/v1/BinaryParameters(Pid='%s',Id='%s')",
		url.QueryEscape(pid),
		url.QueryEscape(id))

	log.Debug().Msgf("Getting binary parameter %s/%s", pid, id)

	resp, err := pd.exe.ExecGetRequest(path, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get binary parameter failed with response code = %d", resp.StatusCode)
	}

	body, err := pd.exe.ReadRespBody(resp)
	if err != nil {
		return nil, err
	}

	var result struct {
		D BinaryParameter `json:"d"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result.D, nil
}

// CreateStringParameter creates a new string parameter
func (pd *PartnerDirectory) CreateStringParameter(param StringParameter) error {
	body := map[string]string{
		"Pid":   param.Pid,
		"Id":    param.ID,
		"Value": param.Value,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	log.Debug().Msgf("Creating string parameter %s/%s", param.Pid, param.ID)

	resp, err := pd.exe.Exec("POST", "/api/v1/StringParameters",
		bytes.NewReader(bodyJSON), map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create string parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UpdateStringParameter updates an existing string parameter
func (pd *PartnerDirectory) UpdateStringParameter(param StringParameter) error {
	body := map[string]string{"Value": param.Value}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	path := fmt.Sprintf("/api/v1/StringParameters(Pid='%s',Id='%s')",
		url.QueryEscape(param.Pid),
		url.QueryEscape(param.ID))

	log.Debug().Msgf("Updating string parameter %s/%s", param.Pid, param.ID)

	resp, err := pd.exe.Exec("PUT", path,
		bytes.NewReader(bodyJSON), map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update string parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// DeleteStringParameter deletes a string parameter
func (pd *PartnerDirectory) DeleteStringParameter(pid, id string) error {
	path := fmt.Sprintf("/api/v1/StringParameters(Pid='%s',Id='%s')",
		url.QueryEscape(pid),
		url.QueryEscape(id))

	log.Debug().Msgf("Deleting string parameter %s/%s", pid, id)

	resp, err := pd.exe.Exec("DELETE", path, nil, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete string parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// CreateBinaryParameter creates a new binary parameter
func (pd *PartnerDirectory) CreateBinaryParameter(param BinaryParameter) error {
	body := map[string]string{
		"Pid":         param.Pid,
		"Id":          param.ID,
		"Value":       param.Value,
		"ContentType": param.ContentType,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	log.Debug().Msgf("Creating binary parameter %s/%s", param.Pid, param.ID)

	resp, err := pd.exe.Exec("POST", "/api/v1/BinaryParameters",
		bytes.NewReader(bodyJSON), map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create binary parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UpdateBinaryParameter updates an existing binary parameter
func (pd *PartnerDirectory) UpdateBinaryParameter(param BinaryParameter) error {
	body := map[string]string{
		"Value":       param.Value,
		"ContentType": param.ContentType,
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	path := fmt.Sprintf("/api/v1/BinaryParameters(Pid='%s',Id='%s')",
		url.QueryEscape(param.Pid),
		url.QueryEscape(param.ID))

	log.Debug().Msgf("Updating binary parameter %s/%s", param.Pid, param.ID)

	resp, err := pd.exe.Exec("PUT", path,
		bytes.NewReader(bodyJSON), map[string]string{
			"Content-Type": "application/json",
			"Accept":       "application/json",
		})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update binary parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// DeleteBinaryParameter deletes a binary parameter
func (pd *PartnerDirectory) DeleteBinaryParameter(pid, id string) error {
	path := fmt.Sprintf("/api/v1/BinaryParameters(Pid='%s',Id='%s')",
		url.QueryEscape(pid),
		url.QueryEscape(id))

	log.Debug().Msgf("Deleting binary parameter %s/%s", pid, id)

	resp, err := pd.exe.Exec("DELETE", path, nil, map[string]string{
		"Accept": "application/json",
	})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete binary parameter failed with response code = %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
