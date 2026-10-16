// Package paint is v2's display list: a frozen, flat sequence of drawing
// commands per compositing layer. It sits directly above frame and below
// surface, so it may use frame geometry but must not import surface, raster, or
// any platform code.
//
// The one design fact that matters downstream: a LayerDL is frozen once
// published. Raster workers read published lists concurrently with no lock and
// no copy, and that is only sound because nothing can mutate the slab after
// publication.
package paint
