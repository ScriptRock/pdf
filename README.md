# PDF Reader

A simple Go library which enables reading PDF files text content.
Fork tree:
- https://github.com/ledongthuc/pdf
- https://github.com/rsc/pdf

`Reader.GetText` returns the text content annotated with text size and weight information.
Text is returned in stream order - irrespectve of where it appears on the page, the returned
text order is how it appears in the PDF stream.

Attempts are made to separate text blocks that are displayed in separate blocks in the PDF as
separate paragraphs.

e.g. with tabular PDF content:

| Col 1 header        | Col 2 header        |
| ------------------- | ------------------- |
| Text in row 1 col 1 | Text in row 1 col 2 |
| Text in row 2 col 1 | Text in row 2 col 2 |

`Reader.GetText` returns content as:

```
Col 1 header

Col 2 header

Text in row 1 col 1

Text in row 1 col 2

Text in row 2 col 1

Text in row 2 col 2
```