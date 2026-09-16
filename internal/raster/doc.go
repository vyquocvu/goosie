// Package raster turns frozen paint lists into tile pixels. It owns the glyph
// atlas, the per-tile CPU rasterizer, the worker pool that keeps rasterization
// off the UI thread, and the Scheduler that sequences one frame.
//
// raster sits at the top of the v2 frame-path graph: frame <- paint <- surface
// <- raster. Nothing imports raster except a command binary and the tests, which
// is why it may depend on everything below it without creating a cycle.
package raster
