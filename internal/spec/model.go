// Package spec — Runbook model (YAML → Go structs)
// All execution logic uses these types.
package spec

import (
	"time"
)

// Runbook — a declarative automation recipe (analogous to Ansible Playbook / Rundeck Job)
type Runbook struct {
	APIVersion string      `yaml:"apiVersion" json:"apiVersion"`
	Kind       string      `yaml:"kind"       json:"kind"`
	Metadata   Metadata    `yaml:"metadata"  json:"metadata"`
	Spec       RunbookSpec `yaml:"spec"      json:"spec"`
}

type Metadata struct {
	Name  string `yaml:"name"  json:"name"`
	Owner string `yaml:"owner" json:"owner"` // runbook author (separation of duties)
}

type RunbookSpec struct {
	Trigger TriggerConfig `yaml:"trigger" json:"trigger"`
	Steps   []Step        `yaml:"steps"   json:"steps"`
}

// TriggerConfig — alert matching rules and storm protection
type TriggerConfig struct {
	Alert       string `yaml:"alert"       json:"alert"`       // alert name (e.g. "DiskUsageHigh")
	Severity    string `yaml:"severity"    json:"severity"`    // critical / warning / info
	Concurrency int    `yaml:"concurrency" json:"concurrency"` // max concurrent runs (storm protection)
	Dedup       string `yaml:"dedup"       json:"dedup"`       // duration: "5m", "1h"
	Throttle    string `yaml:"throttle"    json:"throttle"`    // global rate limit
}

// Step — a single runbook step
// design: each step is idempotent (step_id is stable from YAML)
type Step struct {
	ID        string       `yaml:"id"                    json:"id"`
	Action    string       `yaml:"action"                json:"action"`  // exec / http / script / notify
	Exec      string       `yaml:"exec,omitempty"        json:"exec"`    // shell command
	URL       string       `yaml:"url,omitempty"         json:"url"`     // for action: http
	Script    string       `yaml:"script,omitempty"      json:"script"`  // path to script for action: script
	Timeout   string       `yaml:"timeout,omitempty"     json:"timeout"` // duration: "5s", "30s"
	RunAs     string       `yaml:"runas,omitempty"       json:"runas"`   // user for exec (analogous to Ansible become)
	When      string       `yaml:"when,omitempty"        json:"when"`    // CEL condition
	Retry     *RetryPolicy `yaml:"retry,omitempty"       json:"retry"`
	Approval  *Approval    `yaml:"approval,omitempty"    json:"approval"`
	Undo      *Step        `yaml:"undo,omitempty"        json:"undo"`       // compensation (Saga)
	OnFailure string       `yaml:"on-failure,omitempty"  json:"on-failure"` // rollback / stop / continue
	Notify    *Notify      `yaml:"notify,omitempty"      json:"notify"`     // for action: notify
	Parallel  bool         `yaml:"parallel,omitempty"    json:"parallel"`   // run in parallel with previous step
}

type RetryPolicy struct {
	Max     int           `yaml:"max"     json:"max"`
	Backoff time.Duration `yaml:"backoff" json:"backoff"`
}

type Approval struct {
	Channel       string        `yaml:"channel"                   json:"channel"` // telegram / slack
	Timeout       time.Duration `yaml:"timeout"                   json:"timeout"`
	EscalateAfter time.Duration `yaml:"escalate_after,omitempty"  json:"escalate_after"`
	Show          string        `yaml:"show,omitempty"            json:"show"` // visible payload (do not hide!)
}

type Notify struct {
	Channel string `yaml:"channel" json:"channel"`
	Msg     string `yaml:"msg"     json:"msg"`
}

// Result — the outcome of a single step (journal record)
type Result struct {
	StepID   string      `json:"step_id"`
	Status   Status      `json:"status"` // running / success / failure / skipped / approval-waiting
	ExitCode int         `json:"exit_code"`
	Output   string      `json:"output"`   // stdout
	Error    string      `json:"error"`    // stderr / error message
	Duration string      `json:"duration"` // human-readable execution time
	Data     interface{} `json:"data"`     // arbitrary step data (http response body, script results)
}

type Status string

const (
	StatusRunning   Status = "running"
	StatusSuccess   Status = "success"
	StatusFailure   Status = "failure"
	StatusSkipped   Status = "skipped"
	StatusApproval  Status = "approval-waiting"
	StatusCancelled Status = "cancelled"
)
