# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
import tempfile
import unittest
from pathlib import Path
from compare import dictionary_cases, iter_dictionary_cases, normalize
from html_compare import normalize_html, without_spacing


class ComparisonTests(unittest.TestCase):
    def test_html_vocabulary_mapping(self):
        self.assertEqual(normalize_html('<b class="dsl_b">x &amp; y</b>'), normalize_html('<b>x &amp; y</b>'))
        self.assertEqual(normalize_html('<a class="dsl_ref" href="gdlookup://localhost/C%23?dict=Other">x</a>'),
                         normalize_html('<a class="wu-xref" data-dict="Other" title="Other" href="entry://C%23">x</a>'))
        self.assertEqual(normalize_html('<span class="dsl_p" title="noun">n.</span>'),
                         normalize_html('<span class="wu-p"><abbr class="wu-abbr" title="noun">n.</abbr></span>'))

    def test_html_preserves_content_and_attributes(self):
        for left, right in [('a b', 'ab'), ('<br/>', '<p></p>'),
                            ('<b>x</b>', '<i>x</i>'),
                            ('<a href="entry://a">x</a>', '<a href="entry://b">x</a>'),
                            ('<span class="dsl_p" title="a">x</span>', '<span class="dsl_p" title="b">x</span>'),
                            ('<span class="dsl_lang" lang="ru">x</span>', '<span class="wu-lang" lang="en">x</span>'),
                            ('<span data-other="a">x</span>', '<span data-other="b">x</span>')]:
            self.assertNotEqual(normalize_html(left), normalize_html(right))
        self.assertEqual(without_spacing(normalize_html('a<p></p>b')), without_spacing(normalize_html('a<br/>b')))

    def test_stress_only_removes_verified_duplicate(self):
        gd = '<span class="dsl_stress"><span class="dsl_stress_without_accent">a</span><span class="dsl_stress_with_accent">a\u0301</span></span>'
        go = '<span class="wu-acc">a</span>'
        self.assertEqual(normalize_html(gd), normalize_html(go))
        self.assertNotEqual(normalize_html(gd.replace('a\u0301', 'b\u0301')), normalize_html(go))

    def test_attributes_and_adjacent_text(self):
        gd = {"tag":"ref", "attrs":'target="some word" dict="Other"',
              "children":[{"text":"a"}, {"text":" b"}]}
        go = {"tag":"ref", "attrs":{"dict":"Other", "target":"some word"},
              "children":[{"text":"a b"}]}
        self.assertEqual(normalize(gd), normalize(go))

    def test_preserves_semantic_differences(self):
        self.assertNotEqual(normalize({"text":"a b"}), normalize({"text":"ab"}))
        self.assertNotEqual(normalize({"tag":"b"}), normalize({"tag":"i"}))
        self.assertNotEqual(normalize({"tag":"ref", "attrs":'target="a"'}),
                            normalize({"tag":"ref", "attrs":'target="b"'}))

    def test_spaces_around_assignment(self):
        self.assertEqual(normalize({"tag":"lang", "attrs":"id= 2"}),
                         normalize({"tag":"lang", "attrs":{"id":"2"}}))
        self.assertEqual(normalize({"tag":"ref", "attrs":'target = "C#"'}),
                         normalize({"tag":"ref", "attrs":{"target":"C#"}}))

    def test_empty_root_array_and_null(self):
        self.assertEqual(normalize({"tag":"", "children":None}),
                         normalize({"tag":"", "children":[]}))

    def test_simple_dictionary_limit_and_alternate_headings(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "sample.dsl"
            path.write_text('#NAME "Test"\ngive\n~ up\n\t[b]body[/b]\nnext\n\tbody\n', encoding="utf-8")
            cases = dictionary_cases(path, 1)
            self.assertEqual(len(cases), 1)
            self.assertEqual(cases[0]["headings"], ["give", "~ up"])
            self.assertEqual(cases[0]["body"], "[b]body[/b]")
            self.assertEqual(len(dictionary_cases(path, 0)), 2)

    def test_streaming_batch_boundaries(self):
        from itertools import islice
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "sample.dsl"
            path.write_text("".join(f"word{i}\n\tbody{i}\n" for i in range(7)), encoding="utf-8")
            stream = iter_dictionary_cases(path)
            batches = []
            while batch := list(islice(stream, 3)):
                batches.append(batch)
            self.assertEqual([len(b) for b in batches], [3, 3, 1])
            self.assertEqual([c["id"] for b in batches for c in b], [str(i) for i in range(1, 8)])


if __name__ == "__main__":
    unittest.main()
