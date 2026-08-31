package expr

import (
	"testing"
)

func TestSimpleComparison(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"exit_code": int64(0),
				"output":    "all good",
			},
		},
	}

	result, err := e.EvalBool("steps.check.exit_code == 0", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for exit_code == 0")
	}

	result, err = e.EvalBool("steps.check.exit_code != 0", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if result {
		t.Error("expected false for exit_code != 0")
	}
}

func TestStringContains(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"output": "Disk usage: 85%",
			},
		},
	}

	result, err := e.EvalBool("steps.check.output.startsWith(\"Disk\")", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for startsWith 'Disk'")
	}
}

func TestEnvVars(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"env": map[string]interface{}{
			"HOME": "/home/user",
			"USER": "test",
		},
	}

	result, err := e.EvalBool("env.USER == \"test\"", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for env.USER == 'test'")
	}
}

func TestRunMetadata(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"run": map[string]interface{}{
			"name": "db-check",
		},
	}

	result, err := e.EvalBool("run.name == \"db-check\"", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for run.name == 'db-check'")
	}
}

func TestComplexCondition(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check-usage": map[string]interface{}{
				"exit_code": int64(0),
				"output":    "Disk usage 85%",
			},
		},
		"env": map[string]interface{}{
			"HOME": "/home/user",
		},
	}

	result, err := e.EvalBool(
		`steps["check-usage"].exit_code == 0 && steps["check-usage"].output.startsWith("Disk")`,
		ctx,
	)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for combined condition")
	}
}

func TestNumericComparison(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"exit_code":   int64(0),
				"duration_ms": int64(1500),
			},
		},
	}

	result, err := e.EvalBool("steps.check.duration_ms > 1000", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for duration_ms > 1000")
	}

	result, err = e.EvalBool("steps.check.duration_ms <= 1000", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if result {
		t.Error("expected false for duration_ms <= 1000")
	}
}

func TestTernaryCondition(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"exit_code": int64(0),
			},
		},
	}

	result, err := e.EvalBool("steps.check.exit_code == 0 ? true : false", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true for ternary condition")
	}
}

func TestInvalidExpression(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	err = e.Compile("steps.foo +* bar")
	if err == nil {
		t.Error("expected compile error for invalid expression")
	}
}

func TestTypeMismatch(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"exit_code": int64(42),
			},
		},
	}

	result, err := e.EvalBool("steps.check.exit_code == \"42\"", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v (type mismatch should eval, not error)", err)
	}
	if result {
		t.Error("expected false for int == string")
	}
}

func TestHasMacro(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{"exit_code": int64(0)},
		},
	}

	result, err := e.EvalBool("has(steps.check.exit_code)", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if !result {
		t.Error("expected true: exit_code exists")
	}

	result, err = e.EvalBool("has(steps.check.nonexistent)", ctx)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if result {
		t.Error("expected false: nonexistent does not exist")
	}
}

func TestConditionalOnMissingKey(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{"exit_code": int64(0)},
		},
	}

	result, err := e.EvalBool(
		"has(steps.check.duration_ms) && steps.check.duration_ms > 1000",
		ctx,
	)
	if err != nil {
		t.Fatalf("EvalBool: %v", err)
	}
	if result {
		t.Error("expected false: duration_ms missing, short-circuit to false")
	}
}

func TestCacheReuse(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx1 := map[string]interface{}{
		"env": map[string]interface{}{"MODE": "prod"},
	}
	ctx2 := map[string]interface{}{
		"env": map[string]interface{}{"MODE": "dev"},
	}

	r1, _ := e.EvalBool("env.MODE == \"prod\"", ctx1)
	r2, _ := e.EvalBool("env.MODE == \"prod\"", ctx2)

	if !r1 {
		t.Error("ctx1: expected true")
	}
	if r2 {
		t.Error("ctx2: expected false")
	}
}

func BenchmarkEval(b *testing.B) {
	e, err := New()
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer e.Close()

	ctx := map[string]interface{}{
		"steps": map[string]interface{}{
			"check": map[string]interface{}{
				"exit_code": int64(0),
				"output":    "Disk usage: 85%",
			},
		},
		"env": map[string]interface{}{
			"HOME": "/home/user",
		},
		"run": map[string]interface{}{
			"name": "disk-check",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.EvalBool("steps.check.exit_code == 0 && env.HOME == \"/home/user\"", ctx)
	}
}