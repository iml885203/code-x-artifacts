// This folder is the browser tests, not Go. A go.mod here makes it a module of
// its own, so ./... in the main module does not walk into node_modules.
module pictor.invalid/e2e

go 1.27.1
