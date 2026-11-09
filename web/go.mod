// This folder is the front end, not Go. A go.mod here makes it a module of its
// own, so ./... in the main module (build, vet, test, lint, mod tidy) does not
// walk into node_modules, where some packages ship Go files.
module pictor.invalid/web

go 1.27.1
