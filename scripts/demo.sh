#!/usr/bin/env bash
# demo.sh — Simulates gpu-metrics output across three environments.
# No GPU hardware or NVIDIA driver needed. Runs on any machine.
# Usage: ./scripts/demo.sh [standalone|slurm|flux|all]

set -euo pipefail

BOLD="\033[1m"
DIM="\033[2m"
CYAN="\033[36m"
GREEN="\033[32m"
YELLOW="\033[33m"
RESET="\033[0m"

# Simulated gpu-metrics table output (4x A100 80GB)
table_header() {
    printf "%-5s %-20s %6s %6s %10s %10s %6s %6s %10s %10s %10s %10s\n" \
        "GPU" "Name" "Util%" "Mem%" "MemUsed" "MemTotal" "Temp" "Power" \
        "PCIeTx" "PCIeRx" "NVLTx" "NVLRx"
    echo "---   ----                 -----  -----  ---------  ---------  -----  -----  ---------  ---------  ---------  ---------"
}

gpu0() { printf "%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n" 0 "NVIDIA A100-SXM4-80G" 87 72 57344 81920 62 285 12800 11200 22400 21800; }
gpu1() { printf "%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n" 1 "NVIDIA A100-SXM4-80G" 92 85 69632 81920 67 310 14200 13100 23100 22500; }
gpu2() { printf "%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n" 2 "NVIDIA A100-SXM4-80G" 45 38 31104 81920 51 195 8400 7200 15600 14900; }
gpu3() { printf "%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n" 3 "NVIDIA A100-SXM4-80G" 12 8 6554 81920 38 72 1200 800 2100 1800; }

demo_standalone() {
    echo -e "${BOLD}${CYAN}=== Standalone (bare metal) ===${RESET}"
    echo -e "${DIM}\$ gpu-metrics${RESET}"
    echo ""
    table_header
    gpu0; gpu1; gpu2; gpu3
    echo ""
}

demo_slurm() {
    echo -e "${BOLD}${GREEN}=== SLURM Job ===${RESET}"
    echo -e "${DIM}\$ srun --gres=gpu:2 gpu-metrics${RESET}"
    echo ""
    echo "SLURM Job 98432 (llm-finetune) — node dgx-node-03, rank 0, gpus [0,1]"
    echo ""
    table_header
    gpu0; gpu1
    echo ""
}

demo_slurm_json() {
    echo -e "${BOLD}${GREEN}=== SLURM Job (JSON) ===${RESET}"
    echo -e "${DIM}\$ srun --gres=gpu:2 gpu-metrics --format json${RESET}"
    echo ""
    cat <<'EOF'
{
  "slurm": {
    "JobID": "98432",
    "JobName": "llm-finetune",
    "Partition": "gpu",
    "NodeList": "dgx-node-[03-04]",
    "NodeName": "dgx-node-03",
    "NumNodes": 2,
    "NumTasks": 4,
    "ProcID": 0,
    "LocalID": 0,
    "GPUs": "0,1"
  },
  "devices": [
    {
      "Index": 0, "UUID": "GPU-a1b2c3d4-0000-1111-2222-333344445555",
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 87, "MemoryUtilization": 72,
      "MemoryUsedMiB": 57344, "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 62, "PowerDrawWatts": 285, "PowerLimitWatts": 400,
      "PCIeTxKBps": 12800, "PCIeRxKBps": 11200,
      "NVLinkTxMBps": 22400, "NVLinkRxMBps": 21800
    },
    {
      "Index": 1, "UUID": "GPU-e5f6a7b8-0000-1111-2222-333344445555",
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 92, "MemoryUtilization": 85,
      "MemoryUsedMiB": 69632, "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 67, "PowerDrawWatts": 310, "PowerLimitWatts": 400,
      "PCIeTxKBps": 14200, "PCIeRxKBps": 13100,
      "NVLinkTxMBps": 23100, "NVLinkRxMBps": 22500
    }
  ]
}
EOF
    echo ""
}

demo_flux() {
    echo -e "${BOLD}${YELLOW}=== Flux Job ===${RESET}"
    echo -e "${DIM}\$ flux run -N2 -g 2 gpu-metrics${RESET}"
    echo ""
    echo "Flux Job f23r45t — task rank 0, local rank 0, gpus [2,3]"
    echo ""
    table_header
    gpu2; gpu3
    echo ""
}

demo_flux_json() {
    echo -e "${BOLD}${YELLOW}=== Flux Job (JSON) ===${RESET}"
    echo -e "${DIM}\$ flux run -N2 -g 2 gpu-metrics --format json${RESET}"
    echo ""
    cat <<'EOF'
{
  "flux": {
    "JobID": "f23r45t",
    "TaskRank": 0,
    "LocalID": 0,
    "NumTasks": 8,
    "NumNodes": 2,
    "URI": "local:///run/flux/local",
    "GPUs": "2,3"
  },
  "devices": [
    {
      "Index": 2, "UUID": "GPU-c9d0e1f2-0000-1111-2222-333344445555",
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 45, "MemoryUtilization": 38,
      "MemoryUsedMiB": 31104, "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 51, "PowerDrawWatts": 195, "PowerLimitWatts": 400,
      "PCIeTxKBps": 8400, "PCIeRxKBps": 7200,
      "NVLinkTxMBps": 15600, "NVLinkRxMBps": 14900
    },
    {
      "Index": 3, "UUID": "GPU-34567890-0000-1111-2222-333344445555",
      "Name": "NVIDIA A100-SXM4-80GB",
      "GPUUtilization": 12, "MemoryUtilization": 8,
      "MemoryUsedMiB": 6554, "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 38, "PowerDrawWatts": 72, "PowerLimitWatts": 400,
      "PCIeTxKBps": 1200, "PCIeRxKBps": 800,
      "NVLinkTxMBps": 2100, "NVLinkRxMBps": 1800
    }
  ]
}
EOF
    echo ""
}

demo_comparison() {
    echo -e "${BOLD}=== Cross-Environment Comparison ===${RESET}"
    echo ""
    echo "Notice: identical metric fields across all environments."
    echo "The only difference is the scheduler context block (slurm/flux/none)."
    echo "This enables direct GPU performance comparison across HPC and cloud."
    echo ""
    demo_slurm_json
    echo "---"
    echo ""
    demo_flux_json
}

case "${1:-all}" in
    standalone)  demo_standalone ;;
    slurm)       demo_slurm; demo_slurm_json ;;
    flux)        demo_flux; demo_flux_json ;;
    comparison)  demo_comparison ;;
    all)
        demo_standalone
        echo "================================================================"
        echo ""
        demo_slurm
        echo "================================================================"
        echo ""
        demo_flux
        echo "================================================================"
        echo ""
        demo_comparison
        ;;
    *) echo "Usage: $0 [standalone|slurm|flux|comparison|all]" ;;
esac
