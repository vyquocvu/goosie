#!/usr/bin/env python3
"""Compare Goosie renders against Playwright (Chromium) reference renders."""

import os
import sys
from pathlib import Path
from PIL import Image, ImageChops, ImageDraw, ImageFont
import json

def compare_images(img1_path, img2_path, diff_path, threshold=30):
    """Compare two images and return similarity percentage."""
    try:
        img1 = Image.open(img1_path).convert('RGB')
        img2 = Image.open(img2_path).convert('RGB')
        
        # Resize to match if needed
        if img1.size != img2.size:
            img2 = img2.resize(img1.size, Image.Resampling.LANCZOS)
        
        # Calculate pixel difference
        diff = ImageChops.difference(img1, img2)
        
        # Convert to grayscale for analysis
        diff_gray = diff.convert('L')
        
        # Count pixels above threshold
        pixels = diff_gray.load()
        width, height = diff_gray.size
        total_pixels = width * height
        different_pixels = 0
        
        for y in range(height):
            for x in range(width):
                if pixels[x, y] > threshold:
                    different_pixels += 1
        
        similarity = 100.0 * (1.0 - different_pixels / total_pixels)
        
        # Create visual diff
        diff_visual = ImageChops.difference(img1, img2)
        # Enhance differences
        from PIL import ImageEnhance
        enhancer = ImageEnhance.Contrast(diff_visual)
        diff_visual = enhancer.enhance(5.0)
        diff_visual.save(diff_path)
        
        return similarity
    except Exception as e:
        print(f"Error comparing {img1_path} vs {img2_path}: {e}", file=sys.stderr)
        return 0.0

def main():
    goosie_dir = Path('/tmp/goosie-renders')
    playwright_dir = Path('/tmp/playwright-renders')
    diff_dir = Path('/tmp/render-diffs')
    report_path = Path('/tmp/render-comparison-report.json')
    
    diff_dir.mkdir(parents=True, exist_ok=True)
    
    results = []
    total_similarity = 0.0
    passing = 0
    failing = 0
    
    # Find all PNG files in goosie renders
    for goosie_png in sorted(goosie_dir.rglob('*.png')):
        relative_path = goosie_png.relative_to(goosie_dir)
        playwright_png = playwright_dir / relative_path
        diff_png = diff_dir / relative_path
        
        if not playwright_png.exists():
            print(f"Missing Playwright render: {relative_path}", file=sys.stderr)
            continue
        
        diff_png.parent.mkdir(parents=True, exist_ok=True)
        
        similarity = compare_images(goosie_png, playwright_png, diff_png)
        total_similarity += similarity
        
        status = 'PASS' if similarity >= 90.0 else 'FAIL'
        if status == 'PASS':
            passing += 1
        else:
            failing += 1
        
        results.append({
            'file': str(relative_path),
            'similarity': round(similarity, 2),
            'status': status,
            'goosie': str(goosie_png),
            'playwright': str(playwright_png),
            'diff': str(diff_png),
        })
        
        print(f"{status:4s} {similarity:6.2f}%  {relative_path}")
    
    avg_similarity = total_similarity / len(results) if results else 0.0
    
    report = {
        'summary': {
            'total': len(results),
            'passing': passing,
            'failing': failing,
            'average_similarity': round(avg_similarity, 2),
            'target': 90.0,
        },
        'results': results,
    }
    
    with open(report_path, 'w') as f:
        json.dump(report, f, indent=2)
    
    print(f"\n{'='*70}")
    print(f"Total: {len(results)} | Passing: {passing} | Failing: {failing}")
    print(f"Average similarity: {avg_similarity:.2f}%")
    print(f"Report: {report_path}")
    
    return 0 if avg_similarity >= 90.0 else 1

if __name__ == '__main__':
    sys.exit(main())
