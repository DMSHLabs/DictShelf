// SPDX-License-Identifier: GPL-3.0-or-later
// Adapter only. Parser algorithms are unchanged extracted GoldenDict functions.
#include <QtCore>
#include <algorithm>
#include <cstdio>
#include <cwctype>
#include <list>
#include <map>
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
wstring toWString(QString const &s) { return fromQString(s); }
wstring toWString(char const *s) { return fromQString(QString::fromUtf8(s)); }
}
using gd::wchar;
using gd::wstring;
using std::string;
using std::list;
using std::vector;
using std::map;
namespace Folding {
bool isWhitespace(wchar);
bool isPunct(wchar);
wstring apply(wstring const &, bool = false);
#include "vendor/inc_case_folding.hh"
#include "vendor/inc_diacritic_folding.hh"
#include "vendor/folding.inc"
#include "vendor/language_folding.inc"
}
class LangCoder {
public:
  static quint32 code2toInt(const char code[2]) { return (quint32(code[1]) << 8) + quint32(code[0]); }
  static QString intToCode2(quint32);
  static quint32 findIdForLanguage(wstring const &);
};
#include "vendor/language.inc"
namespace Utf8 {
string encode(wstring const &s) { return gd::toQString(s).toUtf8().toStdString(); }
wstring decode(string const &s) { return gd::fromQString(QString::fromUtf8(s.data(), s.size())); }
bool isspace(int c) { return c == ' ' || c == '\f' || c == '\n' || c == '\r' || c == '\t' || c == '\v'; }
}
#include "vendor/html_helpers.inc"
namespace Qt4x5 { namespace Url {
QString ensureLeadingSlash(QString const &s) { return s.startsWith('/') ? s : '/' + s; }
void setQueryItems(QUrl &url, QList<QPair<QString, QString>> const &items) {
  QUrlQuery query; query.setQueryItems(items); url.setQuery(query);
}
}}
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
#include "vendor/dsl_language.inc"
}}
using namespace Dsl::Details;
class DslDictionary {
public:
  map<string, string> abrv;
  wstring currentHeadword;
  int articleNom = 0, optionalPartNom = 0, excludedMedia = 0;
  QString source;
  string getId() const { return "comparison"; }
  string getName() const { return "comparison"; }
  QString getMainFilename() const { return source; }
  string processNodeChildren(ArticleDom::Node const &);
  string nodeToHtml(ArticleDom::Node const &);
};
#include "vendor/renderer.inc"
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
    QJsonObject record{{"id", c["id"]}, {"keys", keys}, {"tree", node(dom.root)}};
    if (app.arguments().contains("--html")) {
      DslDictionary renderer;
      renderer.currentHeadword = key;
      renderer.source = c["source"].toString();
      auto abbreviations = c["abbreviations"].toObject();
      for (auto i = abbreviations.begin(); i != abbreviations.end(); ++i)
        renderer.abrv[i.key().toUtf8().toStdString()] = i.value().toString().toUtf8().toStdString();
      // dslToHtml normalizes to NFC before constructing ArticleDom.
      ArticleDom htmlDom(gd::fromQString(gd::toQString(body).normalized(QString::NormalizationForm_C)), "comparison", key);
      auto html = renderer.processNodeChildren(htmlDom.root);
      record.insert("html", QString::fromUtf8(html.data(), html.size()));
      record.insert("excluded_media", renderer.excludedMedia);
    }
    result.append(record);
  }
  auto bytes = QJsonDocument(result).toJson(QJsonDocument::Compact);
  return std::fwrite(bytes.constData(), 1, bytes.size(), stdout) == size_t(bytes.size()) ? 0 : 2;
}
