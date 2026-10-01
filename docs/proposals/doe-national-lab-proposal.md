# DOE National Laboratory AI Infrastructure Partnership Proposal

## Executive Summary

**Project**: GPU-Aware Autoscaling for DOE HPC Workloads  
**Technology**: keda-gpu-scaler with Volcano NUMA optimization  
**Impact**: Measurable reduction in GPU idle time and improved job throughput  
**Relevance**: Directly applicable to DOE scientific computing workloads

---

## DOE National Laboratory Pain Points

### Current Challenges
1. **GPU Resource Underutilization**: Static allocation leaves GPUs idle between jobs
2. **Job Scheduling Inefficiency**: No NUMA-aware GPU placement means unnecessary PCIe hops
3. **Energy Waste**: Over-provisioned clusters burn power on idle GPUs
4. **Queue Congestion**: Researchers wait for resources that are allocated but not fully used

### Specific DOE Use Cases
- **Oak Ridge Summit/Frontier**: AI training for climate modeling, materials science
- **LLNL Sierra**: Weapons simulation, fusion research
- **Argonne Aurora**: Drug discovery, genomics, climate simulation

---

## Our Solution: keda-gpu-scaler + Volcano Integration

### Core Capabilities
1. **Real-time GPU Metrics**: NVML-based utilization monitoring
2. **NUMA-Aware Placement**: Volcano GPU NUMA plugin integration
3. **Dynamic Scaling**: Automatic pod scaling based on actual GPU demand
4. **Multi-Profile Support**: Pre-configured for HPC workloads (training, inference, data processing)

### Technical Architecture
```
Scientific Workloads → Kubernetes → Volcano Scheduler → keda-gpu-scaler → GPU Nodes
                     ↓
                NUMA-aware GPU placement via Volcano plugin
                     ↓
                Real-time autoscaling based on NVML metrics
```

---

## DOE-Specific Benefits

### Expected Improvements
Based on initial testing and architecture analysis (not yet benchmarked on DOE hardware):
- GPU utilization gains from releasing idle allocations back to the scheduler
- Reduced PCIe latency on multi-GPU jobs via NUMA-local placement
- Lower energy per job from right-sized allocation instead of static over-provisioning

Actual numbers depend on workload mix and cluster configuration — that's partly why we want to benchmark on real DOE infrastructure.

---

## Proposed Collaboration Pathways

### Option 1: CRADA (Cooperative Research & Development Agreement)
- **Joint development** of DOE-specific GPU scheduling profiles
- **Testing and validation** on actual DOE supercomputing workloads
- **Performance benchmarking** against current scheduling systems

### Option 2: DOE Office of Science Visitor Program
- **On-site collaboration** with ORNL/LLNL HPC teams
- **Direct integration** with existing DOE scheduling infrastructure
- **Knowledge transfer** and training programs

### Option 3: Open Source Contribution
- **DOE-specific features** added to keda-gpu-scaler
- **Joint publications** in HPC and AI conferences
- **Community workshops** and training sessions

---

## Success Metrics

### Technical Metrics
- GPU utilization improvement: Target 40%+
- Job completion time reduction: Target 20%+
- Energy efficiency gains: Target 25%+

### Collaboration Outcomes
- Joint technical report documenting benchmark results
- DOE-specific scaling profiles contributed upstream
- Shared findings at relevant HPC conferences

---

## Implementation Timeline

### Phase 1 (Months 1-3): Pilot Deployment
- Deploy keda-gpu-scaler on test cluster
- Integrate with Volcano NUMA plugin
- Benchmark against baseline performance

### Phase 2 (Months 4-6): DOE Workload Optimization
- Develop DOE-specific scaling profiles
- Test with actual scientific workloads
- Performance tuning and optimization

### Phase 3 (Months 7-12): Production Readiness
- Security review and hardening
- Documentation and training materials
- Production deployment planning

---

## Team & Expertise

### Pavan Madduri (Principal Investigator)
- Creator of keda-gpu-scaler (KEDA external gRPC scaler for GPU workloads)
- Volcano contributor — PR #5095 adds GPU NUMA topology awareness to the scheduler
- CNCF Dragonfly community member — distributed systems and P2P model distribution
- Day job: Senior Platform Engineer running Kubernetes at W.W. Grainger

### Contributors
- **venkata22a**: GPU bandwidth metrics and documentation
- **ibobgunardi**: Health probes and test coverage

---

## Next Steps

1. **Technical Deep Dive**: Present to ORNL/LLNL HPC teams
2. **Pilot Planning**: Define test workloads and success criteria
3. **Agreement Selection**: Choose appropriate collaboration mechanism
4. **Resource Allocation**: Secure compute time for testing
5. **Timeline Finalization**: Set specific milestones and deliverables

---

## Contact Information

**Pavan Madduri**  
Senior Platform Engineer & Open Source Contributor  
GitHub: @pmady  
Email: pavan4devops@gmail.com  
LinkedIn: https://linkedin.com/in/pavanmadduri

**Project Repository**: https://github.com/pmady/keda-gpu-scaler  
**Documentation**: https://keda-gpu-scaler.docs.io  
**Whitepaper**: https://github.com/pmady/keda-gpu-scaler/blob/main/docs/cncf-tag-infra/gpu-aware-autoscaling-whitepaper.md

---

