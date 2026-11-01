"""Unit tests for HTML table parser."""

from nepse_ingestion.parsers.html_tables import parse_html_tables, table_to_dicts


HTML = """
<table>
  <thead><tr><th>Symbol</th><th>Price</th></tr></thead>
  <tbody>
    <tr><td>NABIL</td><td>1,200.50</td></tr>
    <tr><td>NICA</td><td>800</td></tr>
  </tbody>
</table>
"""


def test_parse_html_tables():
    tables = parse_html_tables(HTML)
    assert len(tables) == 1
    assert tables[0].headers == ["Symbol", "Price"]
    assert len(tables[0].rows) == 2


def test_table_to_dicts():
    tables = parse_html_tables(HTML)
    dicts = table_to_dicts(tables[0])
    assert dicts[0]["Symbol"] == "NABIL"
    assert dicts[1]["Price"] == "800"
