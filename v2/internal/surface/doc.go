// Package surface is v2's presentation boundary: the window contract the
// platform implements, the event vocabulary that crosses it, and the
// damage-blit composer that assembles tiles into a backing store.
//
// surface depends on frame and paint only. It must never import a platform
// package — the dependency runs the other way, with platform/* satisfying
// surface.Window. That direction is what keeps the composer testable without a
// display server.
package surface
