# OTAF dApp SDK

**Not released yet.** This directory is a placeholder.

A Go SDK for building dApps: applications running *inside* the CU or DU, on the
sub-10 ms control loop, where a decision has no time to leave the node. That is
where scheduling, beam management and spectrum sensing live — work needing PHY
and MAC telemetry that never crosses an interface.

The interface is E3, carrying setup, subscription, indication, control and
report between the node and the dApp hosted on it, and bridging to E2 so dApps,
xApps and rApps coordinate by timescale rather than contend.

dApps are newer than the other two application types. The concept comes from
O-RAN's research group rather than a ratified working-group specification, and
the interface is still settling; this SDK will follow it as it does. Expect the
API to move more than the others until that happens.

It will follow the same shape as the [rApp SDK](../otaf-rapp-sdk/), within the
constraints of running in a real-time path.

Until then, see [what OTAF is](../docs/) for how the three application types
divide the problem.
