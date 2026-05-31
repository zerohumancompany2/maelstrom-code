# Sketch6 Testing Notes

## Intent

Sketch6 has crossed the threshold where architecture changes are happening faster than we can safely reason about by inspection alone.

We now need minimal tests to protect:

- authored statechart behavior
- hydration invariants
- named chunk projection
- workflow durable history updates
- transition semantics
- runtime reconstruction assumptions

## Immediate priorities

1. charts/state transition tests
2. hydrate validation tests
3. assembly plan + named chunk tests
4. transition tool tests
5. weather tool durable workflow history test

## Important runtime caveats

- workflow durable state and session-local projected state can drift if tool implementations only update one side
- binding recovery is a separate concern from workflow state recovery
- provider stub behavior is still not a stable oracle; keep most early tests below provider/runner level
- integrated scenario test should wait until the weather flow reaches a clean terminal condition

## Things to watch

- default/initial state handling when no transition records exist
- workflow history vs session history divergence
- visible vs enabled tools
- chunk projections carrying enough state detail, not just state labels
- reducer logic getting duplicated across runner/tools/chunks

## Next likely test files

- `sketch/sketch6/charts/charts_test.go`
- `sketch/sketch6/hydrate/hydrate_test.go`
- `sketch/sketch6/assembly/types_test.go`
- `sketch/sketch6/tools/transition_test.go`
- `sketch/sketch6/tools/weather_test.go`
- later: `sketch/sketch6/runner/loop_test.go`
