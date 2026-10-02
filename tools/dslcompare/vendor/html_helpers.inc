/* This file is (c) 2008-2012 Konstantin Isakov <ikm@goldendict.org>
 * Part of GoldenDict. Licensed under GPLv3 or later, see the LICENSE file */

namespace Html {
string escape( string const & str )
{
  string result( str );

  for( size_t x = result.size(); x--; )
    switch ( result[ x ] )
    {
      case '&':
        result.erase( x, 1 );
        result.insert( x, "&amp;" );
      break;

      case '<':
        result.erase( x, 1 );
        result.insert( x, "&lt;" );
      break;

      case '>':
        result.erase( x, 1 );
        result.insert( x, "&gt;" );
      break;

      case '"':
        result.erase( x, 1 );
        result.insert( x, "&quot;" );
      break;

      default:
      break;
    }

  return result;
}

}
namespace Filetype {
string simplifyString( string const & str, bool lowercase )
{
  string result;

  size_t beginPos = 0;

  while( beginPos < str.size() && Utf8::isspace( str[ beginPos ] ) )
    ++beginPos;

  size_t endPos = str.size();

  while( endPos && Utf8::isspace( str[ endPos - 1 ] ) )
    --endPos;

  if( endPos <= beginPos )
    return string();

  result.reserve( endPos - beginPos );

  while( beginPos < endPos )
    result.push_back( lowercase ? tolower( str[ beginPos++ ] ) : str[ beginPos++ ] );

  return result;
}

}
