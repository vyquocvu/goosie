"""Unit tests for region scoring, using small synthetic RGB images."""

import unittest

import numpy as np

from testdata.parity import MIN_INK, score_regions


class RegionScoringTests(unittest.TestCase):
    def setUp(self):
        self.reference = np.full((30, 50, 3), 255, dtype=np.uint8)
        self.reference[8:20, 10:35] = 20

    def test_empty_goosie_band_is_reported(self):
        goosie = np.full_like(self.reference, 255)
        _, _, blanks = score_regions(goosie, self.reference)
        self.assertEqual(blanks, [(8, 19)])

    def test_slightly_different_glyph_color_is_not_blank(self):
        goosie = np.full_like(self.reference, 255)
        goosie[8:20, 10:35] = 240
        _, coverage, blanks = score_regions(goosie, self.reference)
        self.assertEqual(blanks, [])
        self.assertGreater(coverage, 0)

    def test_reference_band_below_minimum_ink_is_ignored(self):
        reference = np.full((30, 50, 3), 255, dtype=np.uint8)
        reference[8:18, 10:10 + (MIN_INK - 1) // 10] = 20
        goosie = np.full_like(reference, 255)
        worst_iou, coverage, blanks = score_regions(goosie, reference)
        self.assertIsNone(worst_iou)
        self.assertIsNone(coverage)
        self.assertEqual(blanks, [])


if __name__ == "__main__":
    unittest.main()
