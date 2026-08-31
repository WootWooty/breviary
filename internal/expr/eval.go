// Package expr — CEL expression evaluator for Breviary (spec: internal/expr/)
// Provides boolean condition evaluation with access to step results and env.
// Expressions: `steps["db_check"].exit_code == 0`, `env.HOME`, etc.
package expr

import (
	"fmt"
	"sync"

	"cel.dev/cel-go/cel"
)

// Evaluator компилирует и кеширует CEL-выражения с единым типизированным контекстом
type Evaluator struct {
	mu     sync.RWMutex
	env    *cel.Env
	cache  map[string]cel.Program
	closed bool
}

// New создаёт Evaluator с декларацией переменных контекста
func New() (*Evaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("steps", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("env", cel.MapType(cel.StringType, cel.StringType)),
		cel.Variable("run", cel.MapType(cel.StringType, cel.StringType)),
	)
	if err != nil {
		return nil, fmt.Errorf("cel: new env: %w", err)
	}

	return &Evaluator{
		env:   env,
		cache: make(map[string]cel.Program),
	}, nil
}

// Compile компилирует и кеширует выражение
func (ev *Evaluator) Compile(expr string) error {
	ev.mu.Lock()
	defer ev.mu.Unlock()

	if _, ok := ev.cache[expr]; ok {
		return nil
	}

	ast, issues := ev.env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("cel: compile %q: %w", expr, issues.Err())
	}

	prg, err := ev.env.Program(ast)
	if err != nil {
		return fmt.Errorf("cel: program %q: %w", expr, err)
	}

	ev.cache[expr] = prg
	return nil
}

// EvalBool evaluates выражение и возвращает bool
// context — карта с ключами "steps", "env", "run"
func (ev *Evaluator) EvalBool(expr string, context map[string]interface{}) (bool, error) {
	ev.mu.RLock()
	prg, ok := ev.cache[expr]
	ev.mu.RUnlock()

	if !ok {
		if err := ev.Compile(expr); err != nil {
			return false, err
		}
		ev.mu.RLock()
		prg = ev.cache[expr]
		ev.mu.RUnlock()
	}

	out, _, err := prg.Eval(context)
	if err != nil {
		return false, fmt.Errorf("cel: eval %q: %w", expr, err)
	}

	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("cel: %q evaluated to %T (%v), expected bool", expr, out.Value(), out.Value())
	}

	return b, nil
}

// MustEvalBool eval без ошибки (fail-open: если CEL сломался — выполняем шаг)
func (ev *Evaluator) MustEvalBool(expr string, context map[string]interface{}) bool {
	v, err := ev.EvalBool(expr, context)
	if err != nil {
		return true
	}
	return v
}

// Close освобождает ресурсы
func (ev *Evaluator) Close() {
	ev.mu.Lock()
	defer ev.mu.Unlock()
	ev.closed = true
	ev.cache = nil
	ev.env = nil
}