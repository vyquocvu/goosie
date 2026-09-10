// Package frame holds the geometry, buffer, tile, and plan vocabulary shared by
// the whole v2 frame path. It is the dependency leaf: it imports nothing from
// v2, so it can be built, tested, and reasoned about on its own.
//
// Coordinate spaces, stated once because every bug in a compositor starts with
// one of these being unclear:
//
//   - Rect and RectF are content space: origin at the document top-left.
//   - Viewport is a window into content space, expressed as an offset plus a
//     size, both in device pixels.
//   - A Tile's Bounds are content space; its Pixels are a local buffer whose
//     origin is Bounds.X0, Bounds.Y0. Rasterizing in local coordinates is what
//     makes tile output position-independent and therefore reusable while the
//     viewport scrolls.
package frame
