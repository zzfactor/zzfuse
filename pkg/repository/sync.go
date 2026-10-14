package repository

// This file previously contained global synchronization primitives (TrazrServicesReady channel)
// for coordinating startup between the exporter and extension.
//
// With the dependency injection refactoring, this is no longer needed:
// - TrazrExtension now owns the ServiceResourceManager
// - TrazrExtension implements TrazrServiceProvider interface with WaitForReady()/IsReady()
// - Exporters obtain the SRM from the extension via Host.GetExtensions()
//
// The global channel has been replaced with instance-level synchronization in TrazrExtension.
