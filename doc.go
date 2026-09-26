// Azpim manages Azure Privileged Identity Management (PIM) role assignments
// for Azure resources from the command line: list the roles you are eligible
// for, activate them (now or at a scheduled time), extend or deactivate them,
// and track your requests. It covers management groups, subscriptions and
// resource groups across the whole tenant, in Azure public cloud, Azure US
// Government and Azure China.
//
// Install it with:
//
//	go install github.com/bgs113/azpim@latest
//
// or download a release binary from https://github.com/bgs113/azpim/releases.
// azpim signs in with DefaultAzureCredential, so run 'az login' first.
//
// Usage:
//
//	azpim <command> [flags]
//
// The commands are:
//
//	eligible     list the PIM roles you can activate
//	active       list your active role assignments
//	activate     activate an eligible role
//	extend       request more time on an active role
//	deactivate   deactivate an active role
//	requests     list your activation, extension and deactivation requests
//	version      print the azpim version
//	completion   generate a shell completion script
//
// Use "azpim <command> --help" for a command's flags.
package main
