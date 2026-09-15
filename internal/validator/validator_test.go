package validator

import "testing"

func TestValidatorKeepsFirstErrorPerKey(t *testing.T) {
	validator := New()
	validator.AddError("email", "first")
	validator.AddError("email", "second")
	if validator.Errors["email"] != "first" {
		t.Fatalf("error = %q, want first", validator.Errors["email"])
	}
}

func TestCheckAndIsValid(t *testing.T) {
	validator := New()
	validator.Check(true, "name", "required")
	if !validator.IsValid() {
		t.Fatal("validator should be valid")
	}
	validator.Check(false, "name", "required")
	if validator.IsValid() {
		t.Fatal("validator should be invalid")
	}
}

func TestPermittedValue(t *testing.T) {
	if !PermittedValue("staff", "owner", "staff") {
		t.Fatal("staff should be permitted")
	}
	if PermittedValue("guest", "owner", "staff") {
		t.Fatal("guest should not be permitted")
	}
}

func BenchmarkValidatorCheck(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		validator := New()
		validator.Check(false, "email", "invalid")
		_ = validator.IsValid()
	}
}
