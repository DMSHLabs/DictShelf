/* This file is (c) 2008-2012 Konstantin Isakov <ikm@goldendict.org>
 * Part of GoldenDict. Licensed under GPLv3 or later, see the LICENSE file */

bool isDslWs( wchar ch )
{
  switch( ch )
  {
    case ' ':
    case '\t':
      return true;
    default:
      return false;
  }
}

string DslDictionary::processNodeChildren( ArticleDom::Node const & node )
{
  string result;

  for( ArticleDom::Node::const_iterator i = node.begin(); i != node.end();
       ++i )
    result += nodeToHtml( *i );

  return result;
}
string DslDictionary::nodeToHtml( ArticleDom::Node const & node )
{
  string result;

  if ( !node.isTag )
  {
    result = Html::escape( Utf8::encode( node.text ) );

    // Handle all end-of-line

    string::size_type n;

    // Strip all '\r'
    while( ( n = result.find( '\r' ) ) != string::npos )
      result.erase( n, 1 );

    // Replace all '\n'
    while( ( n = result.find( '\n' ) ) != string::npos )
      result.replace( n, 1, "<p></p>" );

    return result;
  }

  if ( node.tagName == GD_NATIVE_TO_WS( L"b" ) )
    result += "<b class=\"dsl_b\">" + processNodeChildren( node ) + "</b>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"i" ) )
    result += "<i class=\"dsl_i\">" + processNodeChildren( node ) + "</i>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"u" ) )
  {
    string nodeText = processNodeChildren( node );

    if ( nodeText.size() && isDslWs( nodeText[ 0 ] ) )
      result.push_back( ' ' ); // Fix a common problem where in "foo[i] bar[/i]"
                               // the space before "bar" gets underlined.

    result += "<span class=\"dsl_u\">" + nodeText + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"c" ) )
  {
    result += "<font color=\"" + ( node.tagAttrs.size() ?
      Html::escape( Utf8::encode( node.tagAttrs ) ) : string( "c_default_color" ) )
      + "\">" + processNodeChildren( node ) + "</font>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"*" ) )
  {
      string id = "O" + getId().substr( 0, 7 ) + "_" +
                QString::number( articleNom ).toStdString() +
                "_opt_" + QString::number( optionalPartNom++ ).toStdString();
    result += "<span class=\"dsl_opt\" id=\"" + id + "\">" + processNodeChildren( node ) + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"m" ) )
      result += "<div class=\"dsl_m\">" + processNodeChildren( node ) + "</div>";
  else
  if ( node.tagName.size() == 2 && node.tagName[ 0 ] == L'm' &&
       iswdigit( node.tagName[ 1 ] ) )
    result += "<div class=\"dsl_" + Utf8::encode( node.tagName ) + "\">" + processNodeChildren( node ) + "</div>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"trn" ) )
    result += "<span class=\"dsl_trn\">" + processNodeChildren( node ) + "</span>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"ex" ) )
    result += "<span class=\"dsl_ex\">" + processNodeChildren( node ) + "</span>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"com" ) )
    result += "<span class=\"dsl_com\">" + processNodeChildren( node ) + "</span>";
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"s" ) || node.tagName == GD_NATIVE_TO_WS( L"video" ) )
  {
    ++excludedMedia;
    result += "<dsl-media></dsl-media>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"url" ) )
  {
    string link;
    if( !node.tagAttrs.empty() )
    {
      QString attrs = gd::toQString( node.tagAttrs );
      int n = attrs.indexOf( "target=\"" );
      if( n >= 0 )
      {
        QString target = attrs.mid( n + 8 );
        if( target.endsWith( '\"' ) )
          target.chop( 1 );
        link = Html::escape( Filetype::simplifyString( string( target.toUtf8().data() ), false ) );
      }
    }
    if( link.empty() )
      link = Html::escape( Filetype::simplifyString( Utf8::encode( node.renderAsText() ), false ) );

    if( QUrl::fromEncoded( link.c_str() ).scheme().isEmpty() )
      link = "http://" + link;

    QUrl url( QString::fromUtf8( link.c_str() ) );
    if( url.isLocalFile() && url.host().isEmpty() )
    {
      // Convert relative links to local files to absolute ones
      QString name = QFileInfo( getMainFilename() ).absolutePath();
      name += url.toLocalFile();
      QFileInfo info( name );
      if( info.isFile() )
      {
        name = info.canonicalFilePath();
        url.setPath( Qt4x5::Url::ensureLeadingSlash( QUrl::fromLocalFile( name ).path() ) );
        link = string( url.toEncoded().data() );
      }
    }

    result += "<a class=\"dsl_url\" href=\"" + link +"\">" + processNodeChildren( node ) + "</a>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"!trs" ) )
  {
    result += "<span class=\"dsl_trs\">" + processNodeChildren( node ) + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"p") )
  {
    result += "<span class=\"dsl_p\"";

    string val = Utf8::encode( node.renderAsText() );

    // If we have such a key, display a title

    map< string, string >::const_iterator i = abrv.find( val );

    if ( i != abrv.end() )
    {
      string title;

      if ( Utf8::decode( i->second ).size() < 70 )
      {
        // Replace all spaces with non-breakable ones, since that's how
        // Lingvo shows tooltips
        title.reserve( i->second.size() );

        for( char const * c = i->second.c_str(); *c; ++c )
        {
          if ( *c == ' ' || *c == '\t' )
          {
            // u00A0 in utf8
            title.push_back( 0xC2 );
            title.push_back( 0xA0 );
          }
          else
          if( *c == '-' ) // Change minus to non-breaking hyphen (uE28091 in utf8)
          {
            title.push_back( 0xE2 );
            title.push_back( 0x80 );
            title.push_back( 0x91 );
          }
          else
            title.push_back( *c );
        }
      }
      else
        title = i->second;

      result += " title=\"" + Html::escape( title ) + "\"";
    }

    result += ">" + processNodeChildren( node ) + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"'" ) )
  {
    // There are two ways to display the stress: by adding an accent sign or via font styles.
    // We generate two spans, one with accented data and another one without it, so the
    // user could pick up the best suitable option.
    string data = processNodeChildren( node );
    result += "<span class=\"dsl_stress\"><span class=\"dsl_stress_without_accent\">" + data + "</span>"
        + "<span class=\"dsl_stress_with_accent\">" + data + Utf8::encode( wstring( 1, 0x301 ) )
        + "</span></span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"lang" ) )
  {
    result += "<span class=\"dsl_lang\"";
    if( !node.tagAttrs.empty() )
    {
      // Find ISO 639-1 code
      string langcode;
      QString attr = gd::toQString( node.tagAttrs );
      int n = attr.indexOf( "id=" );
      if( n >= 0 )
      {
        int id = attr.mid( n + 3 ).toInt();
        if( id )
          langcode = findCodeForDslId( id );
      }
      else
      {
        n = attr.indexOf( "name=\"" );
        if( n >= 0 )
        {
          int n2 = attr.indexOf( '\"', n + 6 );
          if( n2 > 0 )
          {
            quint32 id = dslLanguageToId( gd::toWString( attr.mid( n + 6, n2 - n - 6 ) ) );
            langcode = LangCoder::intToCode2( id ).toStdString();
          }
        }
      }
      if( !langcode.empty() )
        result += " lang=\"" + langcode + "\"";
    }
    result += ">" + processNodeChildren( node ) + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"ref" ) )
  {
    QUrl url;
    QString attrs;
    if( !node.tagAttrs.empty() )
      attrs = gd::toQString( node.tagAttrs );

    wstring refStr;
    if( !attrs.isEmpty() )
    {
      int n = attrs.indexOf( "target=\"" );
      if( n >= 0 )
      {
        int n_end = attrs.indexOf( '\"', n + 8 );
        QString target = attrs.mid( n, n_end > n ? n_end - n + 1 : -1 );
        attrs.remove( target );
        if( !attrs.isEmpty() )
          attrs = attrs.trimmed();

        target = target.mid( 8 );
        if( target.endsWith( '\"' ) )
          target.chop( 1 );
        refStr = gd::toWString( target );
      }
    }

    url.setScheme( "gdlookup" );
    url.setHost( "localhost" );

    if( refStr.empty() )
      refStr = node.renderAsText();
    normalizeHeadword( refStr );
    url.setPath( Qt4x5::Url::ensureLeadingSlash( gd::toQString( refStr ) ) );
    if( !attrs.isEmpty() )
    {
      QString attr = attrs.remove( '\"' );
      int n = attr.indexOf( '=' );
      if( n > 0 )
      {
        QList< QPair< QString, QString > > query;
        query.append( QPair< QString, QString >( attr.left( n ), attr.mid( n + 1 ) ) );
        Qt4x5::Url::setQueryItems( url, query );
      }
    }

    result += string( "<a class=\"dsl_ref\" href=\"" ) + url.toEncoded().data() +"\">"
              + processNodeChildren( node ) + "</a>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"@" ) )
  {
    // Special case - insided card header was not parsed

    QUrl url;

    url.setScheme( "gdlookup" );
    url.setHost( "localhost" );
    wstring nodeStr = node.renderAsText();
    normalizeHeadword( nodeStr );
    url.setPath( Qt4x5::Url::ensureLeadingSlash( gd::toQString( nodeStr ) ) );

    result += string( "<a class=\"dsl_ref\" href=\"" ) + url.toEncoded().data() +"\">"
              + processNodeChildren( node ) + "</a>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"sub" ) )
  {
    result += "<sub>" + processNodeChildren( node ) + "</sub>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"sup" ) )
  {
    result += "<sup>" + processNodeChildren( node ) + "</sup>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"t" ) )
  {
    result += "<span class=\"dsl_t\">" + processNodeChildren( node ) + "</span>";
  }
  else
  if ( node.tagName == GD_NATIVE_TO_WS( L"br" ) )
  {
    result += "<br />";
  }
  else
  {
    gdWarning( "DSL: Unknown tag \"%s\" with attributes \"%s\" found in \"%s\", article \"%s\".",
               gd::toQString( node.tagName ).toUtf8().data(), gd::toQString( node.tagAttrs ).toUtf8().data(),
               getName().c_str(), gd::toQString( currentHeadword ).toUtf8().data() );

    result += "<span class=\"dsl_unknown\">[" + string( gd::toQString( node.tagName ).toUtf8().data() );
    if( !node.tagAttrs.empty() )
      result += " " + string( gd::toQString( node.tagAttrs ).toUtf8().data() );
    result += "]" + processNodeChildren( node ) + "</span>";
  }

  return result;
}

