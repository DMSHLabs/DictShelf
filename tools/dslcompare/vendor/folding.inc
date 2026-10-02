/* This file is (c) 2008-2012 Konstantin Isakov <ikm@goldendict.org>
 * Part of GoldenDict. Licensed under GPLv3 or later, see the LICENSE file */

bool isWhitespace( wchar ch )
{
  switch( ch )
  {
    case '\n':
    case '\r':
    case '\t':

    case 0x2028: // Zl, LINE SEPARATOR

    case 0x2029: // Zp, PARAGRAPH SEPARATOR

    case 0x0020: // Zs, SPACE
    case 0x00A0: // Zs, NO-BREAK SPACE
    case 0x1680: // Zs, OGHAM SPACE MARK
    case 0x180E: // Zs, MONGOLIAN VOWEL SEPARATOR
    case 0x2000: // Zs, EN QUAD
    case 0x2001: // Zs, EM QUAD
    case 0x2002: // Zs, EN SPACE
    case 0x2003: // Zs, EM SPACE
    case 0x2004: // Zs, THREE-PER-EM SPACE
    case 0x2005: // Zs, FOUR-PER-EM SPACE
    case 0x2006: // Zs, SIX-PER-EM SPACE
    case 0x2007: // Zs, FIGURE SPACE
    case 0x2008: // Zs, PUNCTUATION SPACE
    case 0x2009: // Zs, THIN SPACE
    case 0x200A: // Zs, HAIR SPACE
    case 0x202F: // Zs, NARROW NO-BREAK SPACE
    case 0x205F: // Zs, MEDIUM MATHEMATICAL SPACE
    case 0x3000: // Zs, IDEOGRAPHIC SPACE
      return true;

    default:
      return false;
  }
}

wstring trimWhitespace( wstring const & in )
{
  wchar const * wordBegin = in.c_str();
  wstring::size_type wordSize = in.size();

  // Skip any leading whitespace
  while( *wordBegin && Folding::isWhitespace( *wordBegin ) )
  {
    ++wordBegin;
    --wordSize;
  }

  // Skip any trailing whitespace
  while( wordSize && Folding::isWhitespace( wordBegin[ wordSize - 1 ] ) )
    --wordSize;

  return wstring( wordBegin, wordSize );
}

