// Package spec — модель Runbook (YAML → структуры Go)
// Вся логика исполнения опирается на эти типы.
package spec

import (
	"time"
)

// Runbook — декларативный рецепт автоматизации (аналог Ansible Playbook / Rundeck Job)
type Runbook struct {
	APIVersion string   `yaml:"apiVersion" json:"apiVersion"`
	Kind       string   `yaml:"kind"       json:"kind"`
	Metadata   Metadata  `yaml:"metadata"  json:"metadata"`
	Spec       RunbookSpec `yaml:"spec"    json:"spec"`
}

type Metadata struct {
	Name  string `yaml:"name"  json:"name"`
	Owner string `yaml:"owner" json:"owner"` // автор runbook (separation of duties)
}

type RunbookSpec struct {
	Trigger TriggerConfig `yaml:"trigger" json:"trigger"`
	Steps   []Step        `yaml:"steps"   json:"steps"`
}

// TriggerConfig — правила матчинга алертов и защиты от шторма
type TriggerConfig struct {
	Alert       string `yaml:"alert"       json:"alert"`       // имя алерта (например "DiskUsageHigh")
	Severity    string `yaml:"severity"    json:"severity"`    // critical / warning / info
	Concurrency int    `yaml:"concurrency" json:"concurrency"` // макс. одновременных runs (защита от шторма)
	Dedup       string `yaml:"dedup"       json:"dedup"`       // время: "5m", "1h"
	Throttle    string `yaml:"throttle"    json:"throttle"`    // глобальный лимит частоты
}

// Step — один шаг runbook'а
// design: каждый шаг идемпотентен (step_id стабилен из YAML)
type Step struct {
	ID        string       `yaml:"id"                    json:"id"`
	Action    string       `yaml:"action"                json:"action"`    // exec / http / script / notify
	Exec      string       `yaml:"exec,omitempty"        json:"exec"`      // shell command
	URL       string       `yaml:"url,omitempty"         json:"url"`       // для action: http
	Script    string       `yaml:"script,omitempty"      json:"script"`    // путь к скрипту для action: script
	Timeout   string       `yaml:"timeout,omitempty"     json:"timeout"`   // duration: "5s", "30s"
	RunAs     string       `yaml:"runas,omitempty"       json:"runas"`     // пользователь для exec (аналог Ansible become)
	When      string       `yaml:"when,omitempty"        json:"when"`      // CEL-условие выполнения
	Retry     *RetryPolicy `yaml:"retry,omitempty"       json:"retry"`
	Approval  *Approval    `yaml:"approval,omitempty"    json:"approval"`
	Undo      *Step        `yaml:"undo,omitempty"        json:"undo"`     // компенсация (Saga)
	OnFailure string       `yaml:"on-failure,omitempty"  json:"on-failure"` // rollback / stop / continue
	Notify    *Notify      `yaml:"notify,omitempty"      json:"notify"`   // для action: notify
	Parallel  bool         `yaml:"parallel,omitempty"    json:"parallel"` // запустить параллельно с предыдущим
}

type RetryPolicy struct {
	Max     int           `yaml:"max"     json:"max"`
	Backoff time.Duration `yaml:"backoff" json:"backoff"`
}

type Approval struct {
	Channel        string        `yaml:"channel"                   json:"channel"` // telegram / slack
	Timeout        time.Duration `yaml:"timeout"                   json:"timeout"`
	EscalateAfter  time.Duration `yaml:"escalate_after,omitempty"  json:"escalate_after"`
	Show           string        `yaml:"show,omitempty"            json:"show"` // видимый payload (не скрывать!)
}

type Notify struct {
	Channel string `yaml:"channel" json:"channel"`
	Msg     string `yaml:"msg"     json:"msg"`
}

// Result — результат выполнения одного шага (журнал)
type Result struct {
	StepID   string      `json:"step_id"`
	Status   Status      `json:"status"`   // running / success / failure / skipped / approval-waiting
	ExitCode int         `json:"exit_code"`
	Output   string      `json:"output"`   // stdout
	Error    string      `json:"error"`    // stderr / err message
	Duration string      `json:"duration"` // человекочитаемое время выполнения
	Data     interface{} `json:"data"`     // произвольные данные для шага (http response body, script results)
}

type Status string

const (
	StatusRunning    Status = "running"
	StatusSuccess    Status = "success"
	StatusFailure    Status = "failure"
	StatusSkipped    Status = "skipped"
	StatusApproval   Status = "approval-waiting"
	StatusCancelled  Status = "cancelled"
)