/*
Copyright 2026 The keda-gpu-scaler Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package version exposes build-time version metadata for the project's
// binaries. Version and BuildDate are injected at link time via -ldflags -X
// (see the Makefile); they keep their placeholder values for un-stamped builds
// such as `go run` or `go build` without ldflags.
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the release version, injected at build time (e.g. "v0.5.0").
	Version = "dev"
	// BuildDate is the UTC build date, injected at build time (e.g. "2026-06-23").
	BuildDate = "unknown"
)

// String returns a one-line version summary for the named binary, e.g.
// "keda-gpu-scaler v0.5.0 (go1.26.4, built 2026-06-23)".
func String(name string) string {
	return fmt.Sprintf("%s %s (%s, built %s)", name, Version, runtime.Version(), BuildDate)
}

// Requested reports whether the user asked for version output, either via the
// --version flag (flagSet) or a bare "version" argument as the first non-flag
// argument. Pass flag.Args() for args.
func Requested(flagSet bool, args []string) bool {
	return flagSet || (len(args) > 0 && args[0] == "version")
}
