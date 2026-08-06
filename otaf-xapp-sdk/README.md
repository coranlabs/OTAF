# OTAF xApp SDK

**Not released yet.** This directory is a placeholder.

A Go SDK for building xApps for the O-RAN Near-RT RIC: applications running on
the 10 ms – 1 s control loop, subscribing to E2 indications from the CU and DU,
issuing E2 control, and working inside the A1 policies handed down by the
Non-RT RIC.

It will follow the same shape as the [rApp SDK](../otaf-rapp-sdk/): lifecycle
and health endpoints, configuration with environment overrides, an ingest
pipeline with backpressure and a dead-letter queue, typed platform clients whose
failures classify themselves, offline descriptor validation, and a test harness
with fakes so logic can be tested without a RIC.

Until then, see [what OTAF is](../docs/) for how the three application types
divide the problem, and the [rApp SDK](../otaf-rapp-sdk/) for the model this
will follow.
