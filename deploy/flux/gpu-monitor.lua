--
-- gpu-monitor.lua — Flux shell rc drop-in for automatic GPU metrics collection.
--
-- Background
-- ----------
-- Earlier versions of this integration ran gpu-metrics as a manual
-- companion process alongside the real job, e.g.:
--
--   flux run -N2 -g1 my-training-job & flux run -N2 gpu-metrics --interval 5s --format json > metrics.jsonl
--
-- That pattern requires users to remember to launch (and clean up) a second
-- job by hand, and gives no guarantee the collector starts/stops in step
-- with the workload it's supposed to observe. Following discussion with the
-- Flux developers (flux-framework/flux-core#7679), the recommended pattern
-- is now a job shell coprocess: a helper that a shell plugin starts at
-- shell.start and stops (SIGTERM, escalating to SIGKILL after a timeout)
-- when the job's tasks exit. flux-core >= the release containing
-- flux-framework/flux-core#7723 ships a generic builtin "coprocess" shell
-- plugin plus a coprocess.define() Lua helper for exactly this use case.
--
-- Usage
-- -----
-- Install this file as a Flux shell initrc drop-in, e.g.:
--
--   cp gpu-monitor.lua /etc/flux/shell/lua.d/gpu-monitor.lua
--
-- (or wherever your site's shell initrc searches for drop-ins — see
-- flux-shell-initrc(5)). Once installed, enable it per job with:
--
--   flux run -o gpu-monitor -N2 -g1 ./train.sh
--
-- gpu-metrics starts automatically on every shell rank when the job starts
-- and is sent SIGTERM (already handled cleanly by gpu-metrics' continuous
-- collection loop, see cmd/gpu-metrics/main.go) as soon as the job's tasks
-- complete. Output defaults to one aggregated file per job; override with
-- "-o gpu-monitor.output=..." to change the destination, e.g. a per-node
-- path if per-task output volume is high:
--
--   flux run -o gpu-monitor.output=/tmp/gpu-{{node.id}}.jsonl -N2 -g1 ./train.sh
--
-- Any coprocess.define() field can be overridden the same way, following
-- the usual "-o <name>.<key>=<val>" shell option convention (see
-- flux-shell-options(7) and flux-shell-initrc(5) for the full schema).
--
coprocess.define {
    name = "gpu-monitor",
    command = {"gpu-metrics", "--env", "flux", "--interval", "5s", "--format", "json"},
    output = "gpu-metrics-{{id}}.jsonl",
}
