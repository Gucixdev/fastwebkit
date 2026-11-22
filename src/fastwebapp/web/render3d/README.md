# render3d (TypeScript)

A three.js scene wrapper: renderer, camera, resize handling and a clamped pixel ratio. The size checks are pure
functions and tested without WebGL.

Depends on: `three` (npm).

## Interface

    render3d_scene.ts   Scene3D, Scene3DOptions, isValidSize, clampPixelRatio

## Test

    cd fastwebapp && npm test
