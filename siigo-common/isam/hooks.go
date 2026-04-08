package isam

import (
	"fmt"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// hooks.go — Events/Hooks and Validation for ISAM ORM
//
// Events (like Eloquent observers):
//
//	isam.Clients.BeforeSave(func(r *isam.Row) error {
//	    if r.Get("nombre") == "" {
//	        return fmt.Errorf("nombre is required")
//	    }
//	    return nil
//	})
//
//	isam.Clients.AfterSave(func(r *isam.Row) {
//	    log.Printf("Saved client %s", r.Get("codigo"))
//	})
//
// Validation:
//
//	isam.Clients.Validate("nombre", isam.Required)
//	isam.Clients.Validate("tipo_doc", isam.MaxLen(2))
//	isam.Clients.Validate("email", isam.MatchRegex(`^[^@]+@[^@]+$`))
//
// ---------------------------------------------------------------------------

// HookFn is a hook that can block an operation by returning an error.
type HookFn func(r *Row) error

// AfterHookFn is a hook called after an operation (cannot block).
type AfterHookFn func(r *Row)

// hooks stores registered hooks per table path
type tableHooks struct {
	beforeSave   []HookFn
	afterSave    []AfterHookFn
	beforeDelete []HookFn
	afterDelete  []AfterHookFn
	validators   []fieldValidator
}

var allHooks = map[string]*tableHooks{}

func getHooks(path string) *tableHooks {
	h, ok := allHooks[path]
	if !ok {
		h = &tableHooks{}
		allHooks[path] = h
	}
	return h
}

// ---------------------------------------------------------------------------
// Event registration
// ---------------------------------------------------------------------------

// BeforeSave registers a hook called before Save(). Return error to abort.
func (t *Table) BeforeSave(fn HookFn) {
	getHooks(t.Path).beforeSave = append(getHooks(t.Path).beforeSave, fn)
}

// AfterSave registers a hook called after a successful Save().
func (t *Table) AfterSave(fn AfterHookFn) {
	getHooks(t.Path).afterSave = append(getHooks(t.Path).afterSave, fn)
}

// BeforeDelete registers a hook called before Delete(). Return error to abort.
func (t *Table) BeforeDelete(fn HookFn) {
	getHooks(t.Path).beforeDelete = append(getHooks(t.Path).beforeDelete, fn)
}

// AfterDelete registers a hook called after a successful Delete().
func (t *Table) AfterDelete(fn AfterHookFn) {
	getHooks(t.Path).afterDelete = append(getHooks(t.Path).afterDelete, fn)
}

// runBeforeSave runs all BeforeSave hooks. Returns first error if any.
func runBeforeSave(r *Row) error {
	h, ok := allHooks[r.table.Path]
	if !ok {
		return nil
	}
	for _, fn := range h.beforeSave {
		if err := fn(r); err != nil {
			return fmt.Errorf("before save: %w", err)
		}
	}
	return nil
}

// runAfterSave runs all AfterSave hooks.
func runAfterSave(r *Row) {
	h, ok := allHooks[r.table.Path]
	if !ok {
		return
	}
	for _, fn := range h.afterSave {
		fn(r)
	}
}

// runBeforeDelete runs all BeforeDelete hooks. Returns first error if any.
func runBeforeDelete(r *Row) error {
	h, ok := allHooks[r.table.Path]
	if !ok {
		return nil
	}
	for _, fn := range h.beforeDelete {
		if err := fn(r); err != nil {
			return fmt.Errorf("before delete: %w", err)
		}
	}
	return nil
}

// runAfterDelete runs all AfterDelete hooks.
func runAfterDelete(r *Row) {
	h, ok := allHooks[r.table.Path]
	if !ok {
		return
	}
	for _, fn := range h.afterDelete {
		fn(r)
	}
}

// ClearHooks removes all hooks for this table.
func (t *Table) ClearHooks() {
	delete(allHooks, t.Path)
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// ValidatorFn validates a field value. Returns error if invalid.
type ValidatorFn func(fieldName, value string) error

type fieldValidator struct {
	field     string
	validator ValidatorFn
}

// Validate registers a validation rule for a field. Checked before Save().
func (t *Table) Validate(fieldName string, fn ValidatorFn) {
	h := getHooks(t.Path)
	h.validators = append(h.validators, fieldValidator{field: fieldName, validator: fn})
}

// runValidation runs all registered validators for a row.
func runValidation(r *Row) error {
	h, ok := allHooks[r.table.Path]
	if !ok {
		return nil
	}
	for _, v := range h.validators {
		value := strings.TrimSpace(r.Get(v.field))
		if err := v.validator(v.field, value); err != nil {
			return fmt.Errorf("validation %s: %w", v.field, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Built-in validators (like Laravel's validation rules)
// ---------------------------------------------------------------------------

// Required validates that a field is not empty.
func Required(fieldName, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	return nil
}

// MinLen validates minimum string length.
func MinLen(min int) ValidatorFn {
	return func(fieldName, value string) error {
		if len(value) < min {
			return fmt.Errorf("%s must be at least %d characters", fieldName, min)
		}
		return nil
	}
}

// MaxLen validates maximum string length.
func MaxLen(max int) ValidatorFn {
	return func(fieldName, value string) error {
		if len(value) > max {
			return fmt.Errorf("%s must be at most %d characters", fieldName, max)
		}
		return nil
	}
}

// InList validates that value is one of the allowed values.
func InList(allowed ...string) ValidatorFn {
	return func(fieldName, value string) error {
		for _, a := range allowed {
			if value == a {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of: %s", fieldName, strings.Join(allowed, ", "))
	}
}

// MatchRegex validates that value matches a regex pattern.
func MatchRegex(pattern string) ValidatorFn {
	re := regexp.MustCompile(pattern)
	return func(fieldName, value string) error {
		if value != "" && !re.MatchString(value) {
			return fmt.Errorf("%s does not match pattern %s", fieldName, pattern)
		}
		return nil
	}
}

// Numeric validates that value contains only digits.
func Numeric(fieldName, value string) error {
	if value == "" {
		return nil
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return fmt.Errorf("%s must be numeric", fieldName)
		}
	}
	return nil
}

// DateFormat validates YYYYMMDD date format.
func DateFormat(fieldName, value string) error {
	if value == "" {
		return nil
	}
	if len(value) != 8 {
		return fmt.Errorf("%s must be 8 characters (YYYYMMDD)", fieldName)
	}
	return Numeric(fieldName, value)
}
