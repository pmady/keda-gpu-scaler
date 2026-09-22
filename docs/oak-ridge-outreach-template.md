# Oak Ridge National Laboratory Outreach Template

## LinkedIn/Email Message Template

**Subject:** Mitigating inter-socket PCIe latency for containerized HPC workloads

Hi [Name],

I'm currently leading an AI autoscaling initiative within the CNCF Technical Advisory Group. We've been working on a new architecture to bypass the standard 15-30s Prometheus metric latency, using a DaemonSet to read NVML hardware telemetry directly, so scaling decisions are gated only by KEDA's polling interval.

We recently combined this `keda-gpu-scaler` with NUMA-aware GPU scheduling in Volcano to mitigate inter-socket PCIe latency during multi-node training. Given [ORNL / ALCF]'s push toward containerized HPC, I am exploring how these sub-second, scale-to-zero architectures might optimize the energy footprint for your idle inference clusters.

Would you or one of your infrastructure architects be open to a 15-minute technical critique of this architecture against your exascale constraints?

Best,
Pavan Madduri
Senior Platform Engineer | CNCF Contributor & Dragonfly Member

## Target Contacts

### Primary Targets
1. **Al Geist II** - HPC Operations, GPU scheduling expertise
2. **Scott Atchley** - Advanced Technologies, containerized HPC
3. **Aaron Haun** - HPC Systems, infrastructure architecture
4. **Tom Beck** - Scientific Computing, workload optimization

### Secondary Targets
5. **Ryan Landfield** - HPC Operations, day-to-day infrastructure
6. **Feiyi Wang** - Data and Analytics, AI workloads
7. **Bronson Messer** - Scientific Computing, exascale applications

## Key Technical Talking Points

### Architecture Highlights
- **No scrape delay**: metric age is KEDA's polling interval, vs 15-30s through Prometheus
- **Direct NVML telemetry** via DaemonSet architecture
- **NUMA-aware GPU placement** through Volcano integration
- **Scale-to-zero capability** for idle inference clusters

### DOE-Specific Benefits
- **Energy footprint optimization** for idle clusters
- **Inter-socket PCIe latency mitigation** for multi-node training
- **Containerized HPC support** aligning with ORNL strategy
- **Exascale constraint awareness** in architecture design

### Technical Differentiators
- **Hardware-level metrics** vs software proxy metrics
- **Sub-second response times** for bursty scientific workloads
- **NUMA topology awareness** for memory bandwidth optimization
- **Kubernetes-native integration** with existing schedulers

## Follow-up Strategy

### If No Response (3 days)
```
Hi [Name], following up on my message about GPU autoscaling for containerized HPC workloads. Given ORNL's leadership in exascale computing, I believe the NUMA-aware scheduling approach could provide meaningful energy savings for your inference clusters.

Would you have 15 minutes next week to discuss how this might address your specific infrastructure challenges?
```

### If Interested
- Set up a 15-min call, keep it technical
- Show the NUMA topology diagrams and actual NVML output
- Ask what their current scheduling pain points are

### If Declines
- Ask if there's someone else on the team who'd be a better fit
- Thank them and move on

## Success Metrics

### Week 1 Goals
- 5+ LinkedIn connections established
- 2+ technical briefings scheduled
- 1+ ORNL-specific use case identified

### Week 2 Goals
- Technical briefings completed
- Feedback gathered on architecture
- Pilot deployment discussion initiated

### Long-term Goals
- ORNL pilot deployment
- Joint technical report
- Letter of support
