package spec

import (
	"testing"
)

func TestValidateYAML(t *testing.T) {
	book, err := ValidateYAML("../../examples/db-disk-space.yaml")
	if err != nil {
		t.Fatalf("ValidateYAML failed: %v", err)
	}
	if book.Metadata.Name != "db-disk-space" {
		t.Errorf("expected name db-disk-space, got %s", book.Metadata.Name)
	}
	if len(book.Spec.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(book.Spec.Steps))
	}

	// Проверка деталей
	step0 := book.Spec.Steps[0]
	if step0.ID != "check-usage" {
		t.Errorf("step 0 id: expected check-usage, got %s", step0.ID)
	}
	if step0.Action != "exec" {
		t.Errorf("step 0 action: expected exec, got %s", step0.Action)
	}
	if step0.Timeout != "10s" {
		t.Errorf("step 0 timeout: expected 10s, got %s", step0.Timeout)
	}

	step1 := book.Spec.Steps[1]
	if step1.Retry == nil {
		t.Fatal("expected retry policy on step 1")
	}
	if step1.Retry.Max != 2 {
		t.Errorf("retry max: expected 2, got %d", step1.Retry.Max)
	}

	step2 := book.Spec.Steps[2]
	if step2.RunAs != "root" {
		t.Errorf("runas: expected root, got %s", step2.RunAs)
	}
	if step2.Approval == nil {
		t.Fatal("expected approval on step 2")
	}
	if step2.Approval.Channel != "telegram" {
		t.Errorf("approval channel: expected telegram, got %s", step2.Approval.Channel)
	}
	if step2.OnFailure != "rollback" {
		t.Errorf("on-failure: expected rollback, got %s", step2.OnFailure)
	}
}

func TestValidateInvalidYAML(t *testing.T) {
	// Проверка: невалидный YAML (не тот kind)
	bad := []byte(`apiVersion: breviary.io/v1
kind: NotRunbook
metadata:
  name: test
spec:
  steps: []`)
	_, err := Validate(bad)
	if err == nil {
		t.Fatal("expected error for invalid kind, got nil")
	}
}

func TestValidateMissingName(t *testing.T) {
	bad := []byte(`apiVersion: breviary.io/v1
kind: Runbook
metadata:
  name: ""
spec:
  steps:
    - id: test
      action: exec
      exec: echo hello`)
	_, err := Validate(bad)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestValidateEmptySteps(t *testing.T) {
	bad := []byte(`apiVersion: breviary.io/v1
kind: Runbook
metadata:
  name: test
spec:
  steps: []`)
	_, err := Validate(bad)
	if err == nil {
		t.Fatal("expected error for empty steps, got nil")
	}
}