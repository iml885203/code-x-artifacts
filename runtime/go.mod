// This folder is the runtime's build source, not Go. A go.mod here makes it a
// module of its own, so ./... in the main module does not walk into
// node_modules, where some packages ship Go files.
module pictor.invalid/runtime

go 1.27.1
