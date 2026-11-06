//go:build !simulate

package main

// A normal build runs the fixes it shows. The simulation build replaces this file
// with fixsim_on.go.

const fixSimulation = false

// simulationMark is added to the window title and the report of a simulation build.
const simulationMark = ""

// simulatedCommand, simulatedResult, simulatedPause, simulateFixed and
// simulateRestored are used only by the simulation build.
func simulatedCommand(c string) string { return c }

func simulatedResult(bool) string { return "" }

const simulatedPause = ""

func simulateFixed(*checker, int) {}

func simulateRestored(*checker, int) {}
