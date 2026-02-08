# Goosie Roadmap

This document outlines the planned features and improvements for the Goosie project. The roadmap is organized into phases based on priority and complexity.

## Current Status (v0.7.0)

✅ **Core Foundation**
- HTTP fetching with Go's net/http
- HTML parsing with golang.org/x/net/html
- JavaScript execution with Goja engine
- GUI with Fyne framework
- `console.log()` support
- `document.getElementById()` support
- Window titled "Goosie"
- Body text rendering

✅ **Navigation Features**
- URL bar with entry field and autocomplete placeholder
- Back/Forward navigation buttons with proper state management
- Refresh/Reload button
- Session-based navigation history with branching support
- Bookmark management (add/remove/list with visual indicators)
- Thread-safe state management for concurrent operations

✅ **HTML Rendering**
- Canvas-based HTML renderer module
- Render tree for DOM representation
- Layout engine with box model calculations
- True inline layout engine with line boxes
- Proper text wrapping and line breaking
- White space handling (normal, nowrap, pre, pre-wrap, pre-line)
- Vertical alignment for inline elements
- Inline-block element support
- Support for core HTML elements (h1-h6, p, div, ul, ol, li, a, img)
- Text styling support (bold, italic)
- HTML hierarchy preservation

✅ **DOM API Extensions**
- `document.querySelector()` and `querySelectorAll()`
- `document.getElementsByClassName()`
- `document.getElementsByTagName()`
- Element creation (`document.createElement()`)
- Element manipulation (appendChild, removeChild, replaceChild, insertBefore)
- Event listeners (addEventListener, removeEventListener)

✅ **Browser APIs**
- `window.location` object with query parameter utilities
- `window.history` API with state management
- `setTimeout()` and `setInterval()` with memory leak prevention
- `fetch()` API for AJAX requests
- Local storage API with data validation and versioning
- Session storage API with standardized schema

## Phase 1: Essential Browser Features (v0.5.0)

### UI Improvements
- [x] Status bar showing loading progress
- [x] Error messages for failed page loads
- [x] Tab support for multiple pages
- [x] Settings/preferences dialog

### Enhanced HTML Support
- [x] CSS basic styling support (colors, fonts, sizes)
- [x] Full image rendering (PNG, JPEG, GIF, WebP)
- [x] Interactive link click handling
- [x] Form elements rendering (input, button, textarea)
- [x] Table rendering

## Phase 2: Enhanced JavaScript Support (v0.6.0)

### Enhanced Console
- [x] `console.error()`, `console.warn()`, `console.info()`
- [x] `console.table()` for structured data
- [x] Console panel in the browser UI
- [x] JavaScript error reporting in UI

## Phase 3: Advanced Features (v0.7.0)

### CSS Support
- [x] Full CSS parser
- [x] Box model implementation (margin, padding, border)
- [ ] Flexbox layout
- [ ] Grid layout
- [ ] CSS animations and transitions
- [ ] Media queries for responsive design

### Security & Privacy
- [ ] HTTPS/TLS support
- [ ] Certificate verification
- [ ] Cookie management
- [ ] Content Security Policy (CSP) support
- [ ] Private browsing mode
- [ ] Pop-up blocker

### Performance
- [x] Viewport-based rendering (30-65x faster)
- [x] Display list caching
- [x] Scroll optimization
- [ ] Page caching
- [ ] Concurrent page loading
- [ ] Resource prefetching
- [ ] Memory optimization
- [ ] Lazy loading for images

## Phase 4: Developer Tools (v0.8.0)

### Debugging Tools
- [ ] Inspect element functionality
- [ ] DOM tree viewer
- [ ] Network request inspector
- [ ] JavaScript debugger
- [ ] Console for executing JavaScript
- [ ] Performance profiler

### Developer Features
- [ ] View page source
- [ ] View rendered HTML
- [ ] CSS inspector and live editing
- [ ] JavaScript console with autocomplete
- [ ] Network waterfall chart
- [ ] Storage inspector (cookies, localStorage)

## Phase 5: Modern Web Standards (v1.0.0)

### HTML5 Features
- [ ] Canvas API support
- [ ] SVG rendering
- [ ] Video and audio elements
- [ ] WebSocket support
- [ ] Web Workers
- [ ] Service Workers for offline support

### Advanced JavaScript
- [ ] ES6+ features support
- [ ] Async/await support
- [ ] Promises
- [ ] Modules (import/export)
- [ ] Web APIs (Geolocation, Notifications, etc.)

### Accessibility
- [ ] Screen reader support
- [ ] Keyboard navigation
- [ ] ARIA attributes support
- [ ] High contrast mode
- [ ] Text zoom functionality

## Long-Term Vision (v2.0.0+)

### Platform Expansion
- [ ] Mobile version (Android/iOS)
- [ ] Browser extensions/plugins system
- [ ] Sync across devices
- [ ] Cloud bookmarks

### Advanced Features
- [ ] PDF viewer
- [ ] Built-in download manager
- [ ] Password manager
- [ ] Ad blocker
- [ ] Reader mode
- [ ] Translation support

### Performance & Optimization
- [ ] Multi-process architecture
- [ ] GPU acceleration
- [ ] JIT compilation for JavaScript
- [ ] Advanced caching strategies
- [ ] Progressive Web App (PWA) support

## Community & Ecosystem

### Documentation
- [ ] API documentation for developers
- [ ] Contributing guidelines
- [ ] Code of conduct
- [ ] Tutorial series for extending the browser
- [ ] Architecture deep-dive articles

### Testing
- [ ] Comprehensive unit test coverage (>90%)
- [ ] Integration tests
- [ ] End-to-end tests
- [ ] Performance benchmarks
- [ ] Security audit

### Developer Experience
- [ ] Plugin/extension API
- [ ] Theme system
- [ ] Custom user scripts
- [ ] Import/export settings
- [ ] Command-line interface for automation

## Contributing

We welcome contributions! If you're interested in working on any of these features:

1. Check the [Issues](https://github.com/vyquocvu/goosie/issues) page for open tasks
2. Comment on an issue to claim it
3. Fork the repository and create a feature branch
4. Submit a pull request with your changes

## Versioning

We follow [Semantic Versioning](https://semver.org/):
- **Major version** (v1.0.0, v2.0.0): Breaking changes or major feature releases
- **Minor version** (v0.2.0, v0.3.0): New features, backward compatible
- **Patch version** (v0.1.1, v0.1.2): Bug fixes and minor improvements

## Feedback

Have suggestions for the roadmap? Please:
- Open an issue with your ideas
- Join discussions in existing issues
- Contact the maintainers

---

*Last updated: October 2025 - v0.7.0 includes box model, viewport rendering, and performance optimizations*
