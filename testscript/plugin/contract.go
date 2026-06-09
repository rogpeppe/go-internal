// Package plugin supports extending a testscript run with external plugin
// binaries, and provides the contract and JSON-RPC bridge that those binaries
// implement.
//
// A plugin is an executable named "testscript-plugin-<name>" found on $PATH.
// It provides additional commands and services to a testscript run, and may
// set environment variables. The testscript host discovers, starts and queries
// a plugin lazily the first time a script runs the "plugin <name>" command;
// enable this by calling [Setup].
//
// A plugin communicates with the host over JSON-RPC carried on its standard
// input (requests) and standard output (responses). It may write freely to its
// standard error for logging; the host captures this and reports it through the
// log of the test that is interacting with the plugin, rather than letting it
// interleave on the host's own standard error. A plugin binary's main is
// essentially:
//
//	func main() {
//		if err := plugin.Serve(myPlugin); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// where myPlugin implements [Interface].
//
// IMPORTANT: a plugin must not write anything other than JSON-RPC frames to
// its standard output, or the protocol will be corrupted. Use standard error
// (or the log package, whose default output is standard error) for logging.
package plugin

// Interface is implemented by a plugin. A single plugin process is shared by
// all tests using it, so all methods must be safe for concurrent use.
type Interface interface {
	// Info returns static information about the plugin: the environment
	// variables it requires and provides, and the commands it makes
	// available to test scripts.
	Info() PluginInfo

	// NewTestInstance creates a new instance of the plugin for a single
	// running test. Multiple instances may exist concurrently.
	NewTestInstance(TestParams) (TestInstance, error)

	// Close shuts the plugin down, releasing any global resources.
	Close()
}

// PluginInfo holds static information about a plugin.
type PluginInfo struct {
	// RequiredEnv holds the set of environment variable names that the
	// plugin needs to be passed when creating a test instance. Only these
	// variables are sent to the plugin.
	RequiredEnv map[string]bool

	// ResultEnv holds the set of environment variable names that the
	// plugin is allowed to set in the test environment. The host rejects
	// any attempt to set a variable not in this set.
	ResultEnv map[string]bool

	// Cmds holds the set of commands provided by the plugin, keyed by
	// command name. These commands become available to a test script after
	// it runs the "plugin" command for this plugin.
	Cmds map[string]CmdInfo
}

// TestParams holds the parameters for creating a new test instance.
type TestParams struct {
	// TestName is the name of the test that the instance is for.
	TestName string

	// PluginParamsDir is the absolute path of the directory holding
	// plugin-specific parameters for the test, or empty if none was given.
	PluginParamsDir string

	// Env holds the values of the variables named in
	// [PluginInfo.RequiredEnv], as resolved in the test environment.
	Env map[string]string
}

// CmdInfo holds static information about a command provided by a plugin.
type CmdInfo struct {
	// RequiredEnv holds the set of environment variable names that the
	// command needs passed to it. Only these variables are sent to the
	// plugin when the command is run.
	RequiredEnv map[string]bool

	// WritesOutput indicates whether the command sets
	// standard error and standard output.
	WritesOutput bool
}

// TestInstance represents a running instance of a plugin for a single test.
// Its methods may be called concurrently with those of other instances.
type TestInstance interface {
	// Env returns the environment variables that the instance wants to set
	// in the test environment. Every key must be present in
	// [PluginInfo.ResultEnv].
	Env() map[string]string

	// RunCmd runs one of the commands provided by the plugin. The returned
	// error indicates a failure of the plugin itself (for example, the
	// command could not be started); a failure of the command being run is
	// reported via [CmdResult.Err].
	RunCmd(CmdParams) (CmdResult, error)

	// Close shuts the instance down, releasing any resources held for the
	// test.
	Close()
}

// CmdParams holds the parameters for running a plugin command.
type CmdParams struct {
	// Name is the name of the command being run.
	Name string

	// Args holds the command's arguments (not including the command name).
	Args []string

	// Dir is the absolute path of the working directory the command should
	// run in.
	Dir string

	// Env holds the values of the variables named in the command's
	// [CmdInfo.RequiredEnv], as resolved in the test environment.
	Env map[string]string

	// Stdin holds the standard input for the command.
	Stdin []byte
}

// CmdResult holds the result of running a plugin command.
type CmdResult struct {
	// Stdout and Stderr hold the command's standard output and standard
	// error respectively.
	Stdout []byte
	Stderr []byte

	// Env holds environment variables that the command wants to set in
	// the test environment. Every key must be present in
	// [PluginInfo.ResultEnv].
	Env map[string]string

	// Err, if non-empty, holds the error message from a failure of the
	// command itself (for example a non-zero exit status). It is distinct
	// from the error returned by [TestInstance.RunCmd], which indicates a
	// failure of the plugin.
	Err string
}
