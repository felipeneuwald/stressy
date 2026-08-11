module github.com/felipeneuwald/stressy

// 1.25.0, not the 1.26 this used to declare: golang.org/x/crypto asks for
// 1.25.0 and is the only other input, so this is as low as the floor goes. It
// costs `errors.AsType`, which is 1.26-only, for `errors.As` and a declared
// variable at each call site — against `go install` working for a user whose
// GOTOOLCHAIN is `local` on Go 1.25 (#121).
go 1.25.0

require golang.org/x/crypto v0.55.0
