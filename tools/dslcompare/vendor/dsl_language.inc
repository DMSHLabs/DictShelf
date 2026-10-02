/* This file is (c) 2008-2012 Konstantin Isakov <ikm@goldendict.org>
 * Part of GoldenDict. Licensed under GPLv3 or later, see the LICENSE file */

struct DSLLangCode
{
  int code_id;
  char code[ 3 ]; // ISO 639-1
};

static DSLLangCode LangCodes[] =
{
  { 1, "en" },
  { 1033, "en" },
  { 2, "ru" },
  { 1049, "ru" },
  { 1068, "az" },
  { 1025, "ar" },
  { 1067, "am" },
  { 15, "af" },
  { 1078, "af" },
  { 9, "eu" },
  { 1069, "eu" },
  { 1133, "ba" },
  { 21, "be" },
  { 1059, "be" },
  { 22, "bg" },
  { 1026, "bg" },
  { 19, "hu" },
  { 1038, "hu" },
  { 10, "nl" },
  { 1043, "nl" },
  { 1032, "el" },
  { 1079, "ka" },
  { 13, "da" },
  { 1030, "da" },
  { 16, "id" },
  { 1057, "id" },
  { 1039, "is" },
  { 6, "es" },
  { 7, "es" },
  { 3082, "es" },
  { 1034, "es" },
  { 5, "it" },
  { 1040, "it" },
  { 1087, "kk" },
  { 1595, "ky" },
  { 28, "ch" },
  { 29, "ch" },
  { 1028, "ch" },
  { 2052, "ch" },
  { 30, "la" },
  { 1540, "la" },
  { 1142, "la" },
  { 1062, "lv" },
  { 1063, "lt" },
  { 1086, "ms" },
  { 3, "de" },
  { 26, "de" },
  { 1031, "de" },
  { 32775, "de" },
  { 14, "nb" },
  { 1044, "nb" },
  { 25, "nn" },
  { 2068, "nn" },
  { 20, "pl" },
  { 1045, "pl" },
  { 8, "pt" },
  { 2070, "pt" },
  { 1048, "ro" },
  { 23, "sr" },
  { 3098, "sr" },
  { 1051, "sk" },
  { 1060, "sl" },
  { 17, "sw" },
  { 1089, "sw" },
  { 1064, "tg" },
  { 1092, "tt" },
  { 27, "tr" },
  { 1055, "tr" },
  { 1090, "tk" },
  { 1091, "tz" },
  { 24, "uk" },
  { 1058, "uk" },
  { 11, "fi" },
  { 1035, "fi" },
  { 4, "fr" },
  { 1036, "fr" },
  { 18, "cs" },
  { 1029, "cs" },
  { 12, "sv" },
  { 1053, "sv" },
  { 1061, "et" },
  { 0, "" },
};

string findCodeForDslId( int id )
{
  for( DSLLangCode const * lc = LangCodes; lc->code_id; ++lc )
  {
    if ( id == lc->code_id )
    {
      // We've got a match
      return string( lc->code );
    }
  }
  return string();
}

namespace
{
  void cutEnding( wstring & where, wstring const & ending )
  {
    if ( where.size() > ending.size() &&
         where.compare( where.size() - ending.size(),
                               ending.size(), ending ) == 0 )
      where.erase( where.size() - ending.size() );
  }
}

quint32 dslLanguageToId( wstring const & name )
{
  static wstring newSp( GD_NATIVE_TO_WS( L"newspelling" ) );
  static wstring st( GD_NATIVE_TO_WS( L"standard" ) );
  static wstring ms( GD_NATIVE_TO_WS( L"modernsort" ) );
  static wstring ts( GD_NATIVE_TO_WS( L"traditionalsort" ) );
  static wstring prc( GD_NATIVE_TO_WS( L"prc" ) );

  // Any of those endings are to be removed

  wstring nameStripped = Folding::apply( name );

  cutEnding( nameStripped, newSp );
  cutEnding( nameStripped, st );
  cutEnding( nameStripped, ms );
  cutEnding( nameStripped, ts );
  cutEnding( nameStripped, prc );

  return LangCoder::findIdForLanguage( nameStripped );
}

