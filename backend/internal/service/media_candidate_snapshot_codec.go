package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// The v1 candidate snapshot wire format intentionally keeps the historical
// PascalCase field names of the nested Go structs. Do not replace these DTOs
// with the runtime structs: runtime-only routing state must never leak into a
// durable snapshot.
type resolvedMediaAccountModelV1 struct {
	Provider        string              `json:"Provider"`
	Adapter         string              `json:"Adapter"`
	UpstreamModel   string              `json:"UpstreamModel"`
	NativeAsyncMode NativeAsyncMode     `json:"NativeAsyncMode"`
	RequestMapping  MediaRequestMapping `json:"RequestMapping"`
}

type mediaModelDefinitionV1 struct {
	ID               int64            `json:"ID"`
	ModelID          string           `json:"ModelID"`
	Vendor           string           `json:"Vendor"`
	MediaType        MediaType        `json:"MediaType"`
	Operations       []MediaOperation `json:"Operations"`
	Constraints      json.RawMessage  `json:"Constraints"`
	BillingUnit      string           `json:"BillingUnit"`
	DefaultAdapter   string           `json:"DefaultAdapter"`
	DefaultAsyncMode NativeAsyncMode  `json:"DefaultAsyncMode"`
	Enabled          bool             `json:"Enabled"`
	CreatedAt        time.Time        `json:"CreatedAt"`
	UpdatedAt        time.Time        `json:"UpdatedAt"`
}

type mediaCandidateSnapshotV1 struct {
	AccountID       int64                       `json:"account_id"`
	Platform        string                      `json:"platform"`
	ResolvedModel   resolvedMediaAccountModelV1 `json:"resolved_model"`
	ResolvedRequest json.RawMessage             `json:"resolved_request,omitempty"`
	ModelDefinition *mediaModelDefinitionV1     `json:"model_definition,omitempty"`
}

func encodeMediaCandidateSnapshotV1(candidates []MediaAccountCandidateSnapshot) (json.RawMessage, error) {
	if _, err := validateMediaCandidateSnapshot(candidates); err != nil {
		return nil, err
	}
	wire := make([]mediaCandidateSnapshotV1, len(candidates))
	for index := range candidates {
		wire[index] = mediaCandidateSnapshotToV1(candidates[index])
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func decodeMediaCandidateSnapshotV1(raw json.RawMessage) ([]MediaAccountCandidateSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire []mediaCandidateSnapshotV1
	if err := decoder.Decode(&wire); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple top-level JSON values")
		}
		return nil, err
	}
	candidates := make([]MediaAccountCandidateSnapshot, len(wire))
	for index := range wire {
		candidates[index] = mediaCandidateSnapshotFromV1(wire[index])
	}
	if _, err := validateMediaCandidateSnapshot(candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func mediaCandidateSnapshotToV1(candidate MediaAccountCandidateSnapshot) mediaCandidateSnapshotV1 {
	wire := mediaCandidateSnapshotV1{
		AccountID: candidate.AccountID,
		Platform:  candidate.Platform,
		ResolvedModel: resolvedMediaAccountModelV1{
			Provider:        candidate.ResolvedModel.Provider,
			Adapter:         candidate.ResolvedModel.Adapter,
			UpstreamModel:   candidate.ResolvedModel.UpstreamModel,
			NativeAsyncMode: candidate.ResolvedModel.NativeAsyncMode,
			RequestMapping:  candidate.ResolvedModel.RequestMapping,
		},
		ResolvedRequest: append(json.RawMessage(nil), candidate.ResolvedRequest...),
	}
	if candidate.ModelDefinition == nil {
		return wire
	}
	definition := cloneMediaModelDefinition(*candidate.ModelDefinition)
	if definition.AdapterResolution.IsReady() {
		definition.DefaultAdapter = definition.AdapterResolution.ResolvedAdapter
		definition.DefaultAsyncMode = definition.AdapterResolution.CompatibilityAsyncMode()
	}
	wire.ModelDefinition = &mediaModelDefinitionV1{
		ID:               definition.ID,
		ModelID:          definition.ModelID,
		Vendor:           definition.Vendor,
		MediaType:        definition.MediaType,
		Operations:       append([]MediaOperation(nil), definition.Operations...),
		Constraints:      append(json.RawMessage(nil), definition.Constraints...),
		BillingUnit:      definition.BillingUnit,
		DefaultAdapter:   definition.DefaultAdapter,
		DefaultAsyncMode: definition.DefaultAsyncMode,
		Enabled:          definition.Enabled,
		CreatedAt:        definition.CreatedAt,
		UpdatedAt:        definition.UpdatedAt,
	}
	return wire
}

func mediaCandidateSnapshotFromV1(candidate mediaCandidateSnapshotV1) MediaAccountCandidateSnapshot {
	result := MediaAccountCandidateSnapshot{
		AccountID: candidate.AccountID,
		Platform:  candidate.Platform,
		ResolvedModel: ResolvedMediaAccountModel{
			Provider:        candidate.ResolvedModel.Provider,
			Adapter:         candidate.ResolvedModel.Adapter,
			UpstreamModel:   candidate.ResolvedModel.UpstreamModel,
			NativeAsyncMode: candidate.ResolvedModel.NativeAsyncMode,
			RequestMapping:  candidate.ResolvedModel.RequestMapping,
		},
		ResolvedRequest: append(json.RawMessage(nil), candidate.ResolvedRequest...),
	}
	if candidate.ModelDefinition == nil {
		return result
	}
	result.ModelDefinition = &MediaModelDefinition{
		ID:               candidate.ModelDefinition.ID,
		ModelID:          candidate.ModelDefinition.ModelID,
		Vendor:           candidate.ModelDefinition.Vendor,
		MediaType:        candidate.ModelDefinition.MediaType,
		Operations:       append([]MediaOperation(nil), candidate.ModelDefinition.Operations...),
		Constraints:      append(json.RawMessage(nil), candidate.ModelDefinition.Constraints...),
		BillingUnit:      candidate.ModelDefinition.BillingUnit,
		DefaultAdapter:   candidate.ModelDefinition.DefaultAdapter,
		DefaultAsyncMode: candidate.ModelDefinition.DefaultAsyncMode,
		Enabled:          candidate.ModelDefinition.Enabled,
		CreatedAt:        candidate.ModelDefinition.CreatedAt,
		UpdatedAt:        candidate.ModelDefinition.UpdatedAt,
	}
	return result
}
