package nlu

import (
	"errors"
	"testing"
)

func TestMultiSourceValidationIdentifiesExactFieldWithoutChangingCaseOrInflection(t *testing.T) {
	op, err := Decode([]byte(`{"intent":"multi_action","confidence":"high","arguments":{"steps":[{"key":"a","source":"задача Отчёт","operation":{"intent":"create_task","confidence":"high","arguments":{"title":"Отчёт"}}},{"key":"b","source":"задачу Смета","operation":{"intent":"create_task","confidence":"high","arguments":{"title":"Смета"}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateSources(op, "Добавь задачу Отчёт и задачу Смета")
	var issue ValidationIssue
	if !errors.As(err, &issue) || issue.Code != "source_not_verbatim" || issue.Field != "arguments.steps[0].source" {
		t.Fatal(err)
	}
}

func TestMultiRejectsInvalidGraphsAndAmbiguousKeys(t *testing.T) {
	op := Operation{Intent: "create_task", Confidence: "high", Arguments: []byte(`{"title":"X"}`)}
	step := func(key string, deps ...string) ActionStep {
		return ActionStep{Key: key, Source: "X", DependsOn: deps, Operation: op}
	}
	for _, steps := range [][]ActionStep{
		{step("a")}, {step("a"), step("a")}, {step("a", "b"), step("b")}, {step("a"), step("b", "b")}, {step("a"), step("b", "a", "a")},
		{step("a"), step("b"), step("c"), step("d"), step("e"), step("f")},
	} {
		if (MultiAction{Steps: steps}).Validate() == nil {
			t.Fatal(steps)
		}
	}
	if err := (MultiAction{Steps: []ActionStep{step("a"), step("b", "a")}}).Validate(); err != nil {
		t.Fatal(err)
	}
}
