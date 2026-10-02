// SPDX-License-Identifier: GPL-3.0-or-later
// Adapter only. Parser algorithms are unchanged extracted GoldenDict functions.
#include <QtCore>
#include <algorithm>
#include <cstdio>
#include <cwctype>
#include <list>
#include <string>
#include <vector>
#define THROW_SPEC(x)
#define GD_NATIVE_TO_WS(x) gd::fromQString(QString::fromWCharArray(x)).c_str()
#define GD_FDPRINTF std::fprintf
#define gdWarning(...) std::fprintf(stderr, __VA_ARGS__)
namespace gd {
using wchar = char32_t;
using wstring = std::u32string;
wstring fromQString(QString const &s) {
  auto chars = s.toUcs4();
  return wstring(chars.begin(), chars.end());
}
QString toQString(wstring const &s) { return QString::fromUcs4(s.data(), s.size()); }
}
using gd::wchar;
using gd::wstring;
using std::string;
using std::list;
using std::vector;
namespace Folding {
#include "vendor/folding.inc"
}
namespace Dsl { namespace Details {
void processUnsortedParts(wstring &, bool);
void expandOptionalParts(wstring &, list<wstring> *, size_t = 0, bool = false);
void expandTildes(wstring &, wstring const &);
void unescapeDsl(wstring &);
void normalizeHeadword(wstring &);
bool isAtSignFirst(wstring const &s) {
  // Qt6 equivalent of the original Qt4/5 QRegExp's anchored match.
  static QRegularExpression re("^[ \\t]*(?:\\[[^\\]]+\\][ \\t]*)*@",
                              QRegularExpression::CaseInsensitiveOption);
  return re.match(gd::toQString(s)).hasMatch();
}
#include "vendor/parser.inc"
}}
using namespace Dsl::Details;
QJsonObject node(ArticleDom::Node const &n) {
  if (!n.isTag) return {{"text", gd::toQString(n.text)}};
  QJsonArray children;
  for (auto const &child : n) children.append(node(child));
  return {{"tag", gd::toQString(n.tagName)}, {"attrs", gd::toQString(n.tagAttrs)},
          {"children", children}};
}
int main(int argc, char **argv) {
  QCoreApplication app(argc, argv);
  QFile input;
  if (!input.open(stdin, QIODevice::ReadOnly)) return 2;
  QJsonParseError error;
  auto doc = QJsonDocument::fromJson(input.readAll(), &error);
  if (error.error != QJsonParseError::NoError || !doc.isArray()) return 2;
  QJsonArray result;
  for (auto const &item : doc.array()) {
    auto c = item.toObject();
    wstring key = gd::fromQString(c["key"].toString());
    QJsonArray keys;
    for (auto const &heading : c["headings"].toArray()) {
      wstring title = gd::fromQString(heading.toString());
      bool comment = false;
      stripComments(title, comment);
      expandTildes(title, key);
      processUnsortedParts(title, true);
      list<wstring> expanded;
      expandOptionalParts(title, &expanded);
      for (auto word : expanded) {
        unescapeDsl(word);
        normalizeHeadword(word);
        if (!word.empty()) keys.append(gd::toQString(word));
      }
    }
    wstring body = gd::fromQString(c["body"].toString());
    bool comment = false;
    stripComments(body, comment);
    expandTildes(body, key);
    ArticleDom dom(body, "comparison", key);
    result.append(QJsonObject{{"id", c["id"]}, {"keys", keys}, {"tree", node(dom.root)}});
  }
  auto bytes = QJsonDocument(result).toJson(QJsonDocument::Compact);
  return std::fwrite(bytes.constData(), 1, bytes.size(), stdout) == size_t(bytes.size()) ? 0 : 2;
}
