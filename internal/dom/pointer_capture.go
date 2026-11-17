package dom

func (d *Document) BeginPointerPress() {
	if d == nil {
		return
	}
	d.pointerCaptureMu.Lock()
	d.pointerPressActive = true
	d.pointerCaptureMu.Unlock()
}

func (d *Document) EndPointerPress() {
	if d == nil {
		return
	}
	d.pointerCaptureMu.Lock()
	d.pointerPressActive = false
	d.pendingPointerCapture = nil
	d.releasedPointerCapture = nil
	d.pointerCaptureMu.Unlock()
}

func (d *Document) RequestPointerCapture(target *Node) bool {
	if d == nil || target == nil {
		return false
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	if !d.pointerPressActive {
		return false
	}
	d.pendingPointerCapture = target
	return true
}

func (d *Document) ReleasePointerCapture(target *Node) {
	if d == nil || target == nil {
		return
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	if d.pendingPointerCapture == target {
		d.pendingPointerCapture = nil
	}
	if d.activePointerCapture == target {
		d.activePointerCapture = nil
		d.releasedPointerCapture = target
	}
}

func (d *Document) HasPointerCapture(target *Node) bool {
	if d == nil || target == nil {
		return false
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	return d.activePointerCapture == target
}

func (d *Document) PromotePointerCapture() *Node {
	if d == nil {
		return nil
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	node := d.pendingPointerCapture
	d.pendingPointerCapture = nil
	if node != nil {
		d.activePointerCapture = node
	}
	return node
}

func (d *Document) CapturedPointerNode() *Node {
	if d == nil {
		return nil
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	return d.activePointerCapture
}

func (d *Document) TakeLostPointerCapture() *Node {
	if d == nil {
		return nil
	}
	d.pointerCaptureMu.Lock()
	defer d.pointerCaptureMu.Unlock()
	if d.releasedPointerCapture != nil {
		node := d.releasedPointerCapture
		d.releasedPointerCapture = nil
		return node
	}
	node := d.activePointerCapture
	d.activePointerCapture = nil
	return node
}
