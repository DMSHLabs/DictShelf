// ver. 2.3
'use strict';

if ('undefined' == typeof GDEnhancer) {
    var GDEnhancer = (function() {
        function run(md5Id) {
            function getSum(num) {
                var strNum = num.toString();
                var strLen = strNum.length;
                var sum = 0;
                for (var i = 0; i < strLen; i++) {
                    sum += parseInt(strNum[i]);
                }
                return sum < 10 ? sum : getSum(sum);
            }

            function hashCode(s) {
                return s.split('').reduce(function(a, b) {
                    a = (a << 5) - a + b.charCodeAt(0);
                    return a & a;
                }, 0);
            }

            function getSiblings(node) {
                while ((node = node.nextElementSibling)) {
                    node.classList.add('gde-content');
                }
            }

            function processOptContent(defCol, optTag) {
                var contentWithOptArr = [];
                for (var i = 0; i < defCol.length; i++) {
                    var dicHeadword,
                        defElem = defCol[i];
                    if (defElem.querySelector('.' + optTag)) {
                        defElem.classList.add('gde-opt');
                        dicHeadword = getElement(
                            getElement(
                                defElem,
                                'gddefinition',
                                'parentElement'
                            ),
                            'gdheadword',
                            'previousElementSibling'
                        );
                        contentWithOptArr.push(new Array(dicHeadword, defElem));
                    }
                }
                contentWithOptArr.forEach(function(item) {
                    var optToggler = document.createElement('span');
                    optToggler.className = 'gde-hwbtn gde-btnmin gde-btnplus';
                    item[0].appendChild(optToggler);
                    optToggler.addEventListener('click', function(event) {
                        event.stopPropagation();
                        toggleOpt.call(item[1], optToggler);
                    });
                });
            }

            function toggleOpt(headword) {
                headword.classList.toggle('gde-btnplus');
                this.classList.toggle('gde-opt');
                updatePage();
            }

            function toggleContent(event) {
                event.stopPropagation();
                this.classList.toggle('gde-artexp');
                this.classList.toggle('gde-artcol');
                updatePage();
            }

            function fitImg(evt) {
                if (evt.target.nodeName === 'IMG') {
                    evt.target.classList.toggle('gde-img');
                    updatePage();
                }
            }
            var gdArticle = document.getElementById('gdarticle-' + md5Id),
                gdDictname = gdArticle.getElementsByClassName('gddictname')[0],
                dicRef = document.getElementById('gdarticleref-' + md5Id),
                dicFooter = document.createElement('div'),
                dicFooterCross = document.createElement('span'),
                dicToggler = document.createElement('span'),
                colorArr = [
                    'purple',
                    'deeppurple',
                    'indigo',
                    'blue',
                    'green',
                    'lightgreen',
                    'lime',
                    'yellow',
                    'amber'
                ];
            gdDictname.classList.add('gde-artexp');
            dicFooter.className = 'gde-footer';
            dicFooterCross.className = 'gde-cross';
            dicToggler.className = 'gde-dicname';
            gdDictname.appendChild(dicToggler);
            dicFooter.appendChild(dicFooterCross);
            gdArticle.appendChild(dicFooter);
            gdArticle.classList.add(
                'gde-' + colorArr[getSum(Math.abs(hashCode(md5Id))) - 1]
            );
            getSiblings(gdDictname);
            gdDictname.addEventListener('click', toggleContent);
            dicFooter.addEventListener('click', function(event) {
                toggleContent.call(gdDictname, event);
                GDE.jump(Math.ceil(dicRef.getBoundingClientRect().top), {
                    duration: _duration,
                    callback: afterScrollTimeOut
                });
            });
            var dslDef = gdArticle.getElementsByClassName('dsl_definition');
            if (dslDef.length) {
                processOptContent(dslDef, 'dsl_opt');
            }
            var sdctX = gdArticle.getElementsByClassName('sdct_x');
            if (sdctX.length) {
                processOptContent(sdctX, 'xdxf_ex');
            }
            gdArticle.addEventListener(
                'click',
                function() {
                    gdMakeArticleActive(md5Id);
                    isClicked = true;
                },
                true
            );
            if (gdArticle.getElementsByTagName('img').length) {
                gdArticle.addEventListener('click', fitImg);
            }
            requestAnimationFrame(function() {
                updatePage(gdArticle);
            });
        }

        function getElement(elem, cls, method) {
            while ((elem = elem[method]) && !elem.classList.contains(cls));
            return elem;
        }

        function ready() {
            function check(mutationRecords) {
                var addedNode, currentDicId, artTempl, cachedContent;
                for (var i = 0; i < mutationRecords.length; i++) {
                    for (
                        var j = 0;
                        j < mutationRecords[i].addedNodes.length;
                        j++
                    ) {
                        addedNode = mutationRecords[i].addedNodes[j];
                        if (addedNode.nodeType === 1) {
                            switch (addedNode.localName) {
                                case 'script':
                                    if (
                                        !!(currentDicId = addedNode.innerHTML.split(
                                            /['"]/
                                        )[1]) &&
                                        currentDicId.length === 32
                                    ) {
                                        if (
                                            !selfDeleted &&
                                            selfDictId === currentDicId
                                        ) {
                                            selfDeleted = true;
                                            addedNode.remove();
                                            selfDict.remove();
                                            selfDict = null;
                                            selfDictRef.remove();
                                            selfDictRef = null;
                                        } else if (
                                            microcache.contains(currentDicId) &&
                                            (cachedContent = microcache.get(
                                                currentDicId
                                            ))
                                        ) {
                                            //typeof cachedContent == 'number' ? console.log('cachedContent: ' + cachedContent) : void 0;
                                            if (
                                                cachedContent !== null &&
                                                typeof cachedContent != 'number'
                                            ) {
                                                cachedContent.content.appendChild(
                                                    addedNode
                                                );
                                                delayDicArr.push(currentDicId);
                                                delayDicHistArr.push(
                                                    currentDicId
                                                );
                                                if (
                                                    delayDicHistArr.length <=
                                                        dicCountToLoad ||
                                                    !hasScrollbar
                                                ) {
                                                    //console.time('appendTempl');
                                                    appendTempl(
                                                        delayDicArr.shift()
                                                    );
                                                    //console.timeEnd('appendTempl');
                                                }
                                            }
                                        }
                                    }
                                    break;

                                case 'span':
                                    if (
                                        /^gdarticle(?:ref)?/i.test(
                                            addedNode.className
                                        ) &&
                                        (currentDicId = addedNode.id
                                            .split('-')
                                            .pop()) &&
                                        !~delayDicHistArr.indexOf(currentDicId)
                                    ) {
                                        if (
                                            addedNode.className ===
                                            'gdarticleref'
                                        ) {
                                            artTempl = document.createElement(
                                                'template'
                                            );
                                            artTempl.id =
                                                'template-' + currentDicId;
                                            artTempl.content.appendChild(
                                                addedNode
                                            );
                                            microcache.set(
                                                currentDicId,
                                                artTempl
                                            );
                                        } else if (
                                            microcache.contains(currentDicId)
                                        ) {
                                            microcache
                                                .get(currentDicId)
                                                .content.appendChild(addedNode);
                                        }
                                    }
                            }
                        }
                    }
                }
            }
            var MutationObserver =
                window.MutationObserver || window.WebKitMutationObserver;
            if (!observer) {
                observer = new MutationObserver(check);
                observer.observe(document.body, {
                    childList: true
                });
            }
        }

        function observerDisconnect() {
            observer.disconnect();
            isLoaded = true;
        }

        function appendTempl(dicId) {
            function unwrap(wrapper) {
                var parent = wrapper.parentNode;
                while (wrapper.firstChild) {
                    parent.insertBefore(wrapper.firstChild, wrapper);
                }
                parent.removeChild(wrapper);
            }

            function processDslDef(elem) {
                function isBlank(el) {
                    return (
                        el.nodeType === 3 && el.nodeValue.trim().length === 0
                    );
                }

                function isPureChild(elem) {
                    if (
                        !!elem.previousElementSibling ||
                        !!elem.nextElementSibling
                    )
                        return false;
                    var prevSibling = elem.previousSibling,
                        nextSibling = elem.nextSibling;
                    if (!prevSibling && !nextSibling) return true;
                    if (!prevSibling && nextSibling) {
                        return isBlank(nextSibling);
                    } else if (prevSibling && !nextSibling) {
                        return isBlank(prevSibling);
                    } else return isBlank(prevSibling) && isBlank(nextSibling);
                }

                function getMaxBlank(emptyTag) {
                    for (var parent; emptyTag; ) {
                        parent = emptyTag.parentNode;
                        if (
                            ~blockTags.indexOf(parent.nodeName) ||
                            parent.classList.contains('dsl_definition') ||
                            parent.textContent.trim().length !== 0
                        )
                            return emptyTag;
                        emptyTag = parent;
                    }
                }

                function isBreak(el) {
                    switch (el.nodeType) {
                        case 1:
                            return !!~blockTags.indexOf(el.nodeName);

                        case 3:
                            return !/\S/.test(el.nodeValue)
                                ? (
                                      (el = el.nextSibling),
                                      el ? isBreak(el) : true
                                  )
                                : false;
                    }
                }

                function merge(pureChild) {
                    var parent;
                    do {
                        parent = pureChild.parentNode;
                        if (!~tagArr.indexOf(pureChild.nodeName)) continue;
                        if (pureChild.className.length !== 0) {
                            parent.className +=
                                (parent.className.length !== 0 ? ' ' : '') +
                                pureChild.className;
                        }
                        unwrap(pureChild);
                    } while ((pureChild = parent) && isPureChild(pureChild));
                }
                var elems = elem.getElementsByTagName('p'),
                    elemsLen = elems.length,
                    tagArr = ['B', 'DIV', 'I', 'P', 'SPAN'],
                    toDelArr = [],
                    blockTags = ['DIV', 'P'],
                    toMergeArr = [],
                    toDelEl,
                    pBlank,
                    toDelNextEl;
                for (var i = 0; i < elemsLen; i++) {
                    pBlank = elems[i];
                    if (
                        !pBlank.firstElementChild &&
                        (!pBlank.hasChildNodes() || isBlank(pBlank.firstChild))
                    ) {
                        toDelEl = getMaxBlank(pBlank);
                        toDelNextEl = toDelEl.nextSibling;
                        if (!toDelNextEl || isBreak(toDelNextEl))
                            toDelArr.push(toDelEl);
                    }
                }
                while (toDelArr.length) {
                    toDelArr.pop().remove();
                }
                var allElems = elem.getElementsByTagName('*'),
                    allElemsLen = allElems.length,
                    currentElem;
                for (i = 0; i < allElemsLen; i++) {
                    currentElem = allElems[i];
                    if (
                        currentElem.nextElementSibling ||
                        currentElem.previousElementSibling ||
                        !~tagArr.indexOf(currentElem.nodeName)
                    )
                        continue;
                    if (
                        (!currentElem.firstElementChild ||
                            !isPureChild(currentElem.firstElementChild)) &&
                        isPureChild(currentElem)
                    ) {
                        toMergeArr.push(currentElem);
                    }
                }
                while (toMergeArr.length) {
                    merge(toMergeArr.pop());
                }
            }
            var templ = microcache.get(dicId);
            if (templ) {
                var gdArt = templ.content.querySelector('#gdarticle-' + dicId),
                    gde = gdArt.getElementsByClassName('gde'),
                    toMergeDef = gdArt.getElementsByClassName('dsl_definition'),
                    lsaPlay = gdArt.getElementsByClassName('lsa_play');
                for (var i = gde.length - 1; i >= 0; i--) {
                    var gdeParent = gde[i].parentNode;
                    var gdeAdj = gdeParent.getElementsByClassName('gde');
                    for (var j = gdeAdj.length - 1; j >= 0; j--) {
                        if (j !== 0) i--;
                        unwrap(gdeAdj[j]);
                    }
                    gdeParent.normalize();
                }
                if (toMergeDef.length) {
                    //console.time('processDslDef');
                    for (i = 0; i < toMergeDef.length; i++) {
                        processDslDef(toMergeDef[i]);
                    }
                    var wavElems = gdArt.getElementsByClassName('dsl_s_wav'),
                        wavElemsLen = wavElems.length,
                        audioScript;
                    if (wavElemsLen) {
                        for (i = wavElemsLen - 1; i >= 0; i--) {
                            if (i === 0) {
                                wavElems[0]
                                    .getElementsByTagName('a')[0]
                                    .classList.add('gde-fstwav');
                                continue;
                            } else {
                                audioScript =
                                    wavElems[i].previousElementSibling;
                                if (audioScript.localName === 'script')
                                    audioScript.remove();
                            }
                        }
                    }
                    //console.timeEnd('processDslDef');
                }
                if (lsaPlay.length) {
                    lsaPlay[0]
                        .getElementsByTagName('a')[0]
                        .classList.add('gde-fstwav');
                }
                var clone = document.importNode(templ.content, true);
                document.body.appendChild(clone);
                microcache.set(dicId, null);
                run(dicId);
            }
        }

        function updatePage(art) {
            if ('undefined' != typeof art) {
                art.style.display = 'block';
            }
            requestAnimationFrame(measure);
        }

        function debounce(func, wait) {
            var timeout, args, context, timestamp;
            var later = function() {
                var last = Date.now() - timestamp;
                if (!isScrolling) {
                    //console.log('Scroll Start!')
                    document.body.classList.add('gde-scroll');
                    isScrolling = true;
                }
                if (last < wait) {
                    timeout = setTimeout(later, wait - last);
                } else {
                    timeout = null;
                    func.apply(context, args);
                    return;
                }
            };
            return function onScrollFn() {
                context = this;
                args = arguments;
                timestamp = Date.now();
                if (!timeout) {
                    timeout = setTimeout(later, wait);
                }
            };
        }

        function inView(articleId) {
            var currentOffset = Math.round(currentYOffset);
            var dicIdx = gdToc.indexOf(articleId);
            var dicOffset = microcache.get(gdToc[dicIdx]);
            return (
                currentOffset <= dicOffset && currentOffset + winH > dicOffset
            );
        }
        /*function onResize() {
            if (delayDicArr.length) {
                currentYOffset = window.pageYOffset;
                winH = document.documentElement.clientHeight;
                lastYOffset = 0;
                measure();
            }
        }*/
        // Start
        var GDE = {},
            currentYOffset = 0,
            delay = 1e3 / 15,
            delayDicArr = [],
            delayDicHistArr = [],
            dicCountToLoad = 5,
            isClicked = false,
            isLoaded = false,
            isScrolling = false,
            lastAfterScrollTime = 0,
            lastMeasureTime = 0,
            lastYOffset = 0,
            selfDeleted = false,
            winH = document.documentElement.clientHeight,
            _duration,
            hasScrollbar,
            lineH,
            observer,
            scrollHeight,
            selfDict,
            selfDictId,
            selfDictRef,
            MicroCache = function() {
                var _values = {};
                return {
                    get: function(key) {
                        return _values[key];
                    },
                    contains: function(key) {
                        return key in _values;
                    },
                    set: function(key, value) {
                        _values[key] = value;
                    }
                };
            },
            microcache = new MicroCache(),
            measure = function(time) {
                if (time) {
                    var last = time - lastMeasureTime;
                    if (last < 1e3 / 60) return;
                    lastMeasureTime = time;
                }
                var dicsToc = gdToc,
                    currentDicOffsetTop,
                    currentKeyVal,
                    currentId;
                for (var i = dicsToc.length - 1; i >= 0; i--) {
                    currentId = dicsToc[i];
                    currentDicOffsetTop = document.getElementById(
                        'gdarticleref-' + currentId
                    ).offsetTop;
                    //console.log(currentId + ': ' + currentDicOffsetTop);
                    currentKeyVal = microcache.get(currentId);
                    if (currentDicOffsetTop !== currentKeyVal) {
                        microcache.set(currentId, currentDicOffsetTop);
                    } else break;
                }
                scrollHeight = document.body.scrollHeight;
                hasScrollbar = scrollHeight > winH;
                //console.log('measured!');
                //console.log('delayDicArr.length: ' + delayDicArr.length);
                if (delayDicArr.length && !hasScrollbar)
                    requestAnimationFrame(afterScroll);
            },
            afterScroll = function(time) {
                if (time) {
                    var last = time - lastAfterScrollTime;
                    if (last < 1e3 / 30) return;
                    lastAfterScrollTime = time;
                }
                currentYOffset = window.pageYOffset;
                if (currentYOffset >= lastYOffset && delayDicArr.length) {
                    var lastDicTop;
                    if (
                        (lastDicTop = document.getElementById(
                            'gdarticleref-' + gdToc[gdToc.length - 1]
                        ).offsetTop) &&
                        winH + currentYOffset > lastDicTop - winH /* / 2*/
                    ) {
                        //console.time('appendTempl');
                        appendTempl(delayDicArr.shift());
                        //console.timeEnd('appendTempl');
                        requestAnimationFrame(afterScrollTimeOut);
                    }
                }
                if (gdToc.length > 0) {
                    var currentIdx = _gdFindNextArticleToScrollTo();
                    if (currentIdx > 0) {
                        --currentIdx;
                        if (isClicked) {
                            if (!inView(gdCurrentArticle)) {
                                gdMakeArticleActive(gdToc[currentIdx]);
                                isClicked = false;
                            }
                        } else {
                            gdMakeArticleActive(gdToc[currentIdx]);
                        }
                    }
                }
                document.body.classList.remove('gde-scroll');
                isScrolling = false;
                lastYOffset = currentYOffset;
            },
            afterScrollTimeOut = function() {
                setTimeout(afterScroll, delay);
            },
            onScroll = debounce(function() {
                requestAnimationFrame(afterScroll);
            }, delay);

        /******* init Function *******/
        GDE.init = function() {
            var achtungStyle =
                'font-size:2em;position:fixed;z-index:999;top:0;left:0;box-sizing:border-box;padding:1em;text-align:center;color:#fff;background-color:#8b0000';
            if (gdToc.length) {
                var achtung = document.createElement('div');
                achtung.className = 'gde-achtung';
                achtung.style = achtungStyle;
                achtung.innerHTML =
                    'Put <b style="color:#ffff00">GoldenDict Enhancer</b> at the <b style="color:#ff00ff">TOP</b> of the dictionary list!<br> And try again ;-)';
                document.body.insertBefore(achtung, document.body.firstChild);
                return;
            }
            var cssCheck = document.createElement('div');
            cssCheck.id = 'gde-csscheck';
            cssCheck.style = achtungStyle;
            cssCheck.innerHTML =
                'It seems there is some <b style="color:#ff00ff">problem</b> with the custom <b style="color:#ffff00">article-style.css</b>!';
            document.body.insertBefore(cssCheck, document.body.firstChild);
            var isAchtungHidden = (function() {
                var style = window.getComputedStyle(cssCheck);
                return style.display === 'none';
            })();
            if (!isAchtungHidden) return;
            lineH = (function() {
                var temp = document.createElement('div');
                temp.innerText = 'test';
                temp = document.body.appendChild(temp);
                var ret = temp.clientHeight;
                temp.remove();
                return ret;
            })();
            //console.log(lineH);
            var gdeCss = document.createElement('style');
            gdeCss.innerHTML = '.gdarticle {display: none;}';
            document.head.appendChild(gdeCss);
            window.removeEventListener('click', _gdHandleWordClick, false);
            window.addEventListener('scroll', onScroll);
            //window.addEventListener('resize', onResize);
            var scr = document.currentScript;
            selfDict = getElement(scr, 'gdarticle', 'parentElement');
            selfDictId = selfDict.id.split('-').pop();
            selfDictRef = document.getElementById('gdarticleref-' + selfDictId);
            selfDictRef.style.display = 'none';
            ready();
            var ifLoaded = function(fn) {
                if (document.readyState === 'complete') {
                    return fn();
                }
                document.addEventListener('DOMContentLoaded', fn);
            };
            ifLoaded(observerDisconnect);
            _duration = function(distance) {
                //console.log(distance);
                var duration = Math.sqrt(1e4 * Math.abs(distance) / winH);
                return duration > 350 ? duration : 350;
            };
            /****** patching article-script.js ******/
            _gdGetWordAtPoint = function(elem, x, y) {
                if (
                    1 === elem.nodeType &&
                    ('A' === elem.nodeName ||
                        elem.classList.contains('gddictname'))
                )
                    return null;
                if (3 === elem.nodeType) {
                    var range = elem.ownerDocument.createRange();
                    range.selectNodeContents(elem);
                    for (
                        var currentPos = 0,
                            endPos = range.endOffset,
                            wordStartPos = 0,
                            start = false;
                        currentPos < endPos;
                        currentPos++
                    ) {
                        range.setStart(elem, currentPos);
                        range.setEnd(elem, currentPos + 1);
                        if (
                            /[\s!-#%-\x29,-/:-@\x5B-\x5D^_\x7B-\x7E¡§«¶·»¿;]/.test(
                                range.toString()
                            )
                        ) {
                            if (start) {
                                range.setStart(elem, wordStartPos);
                                range.setEnd(elem, currentPos);
                                start = false;
                                var ret = _gdCheckRange(range, x, y);
                                if (null !== ret) return ret;
                            }
                        } else
                            start ||
                                ((wordStartPos = currentPos), (start = true));
                    }
                    if (start) {
                        range.setStart(elem, wordStartPos);
                        range.setEnd(elem, currentPos);
                        var ret = _gdCheckRange(range, x, y);
                        if (null !== ret) return ret;
                    }
                } else
                    for (var i = 0; i < elem.childNodes.length; i++) {
                        var range = elem.childNodes[
                            i
                        ].ownerDocument.createRange();
                        range.selectNodeContents(elem.childNodes[i]);
                        var rect = range.getBoundingClientRect();
                        range.detach();
                        if (
                            null !== rect &&
                            rect.left <= x &&
                            rect.right >= x &&
                            rect.top <= y &&
                            rect.bottom >= y
                        ) {
                            var result = _gdGetWordAtPoint(
                                elem.childNodes[i],
                                x,
                                y
                            );
                            if (null !== result) return result;
                        }
                    }
                return null;
            };

            var _gdeHandleWordClick = function(event) {
                for (
                    var elem = event.target;
                    elem && !elem.classList.contains('gdarticle');
                    elem = elem.parentNode
                ) {
                    if (
                        /^(?:A|IMG|BODY|HTML)$/i.test(elem.nodeName) ||
                        elem.classList.contains('gddictname')
                    )
                        return;
                }
                var word = _gdGetWordAtPoint(event.target, event.x, event.y);
                if (word !== null) {
                    var charset =
                        '[\u2000-\u206F\u2E00-\u2E7F\u2329\u232A\u27E6-\u27EF\u3008-\u301B\x00-#%-/:-@\x5B-\x60\x7B-~¡§«¶·»¿;]';
                    var regex = new RegExp(
                        '^' + charset + '+|' + charset + '+$',
                        'g'
                    );
                    word = word.replace(regex, '');
                    if (word) alert('handleWordClick://' + word);
                }
            };
            window.addEventListener('click', _gdeHandleWordClick);
            _gdFindNextArticleToScrollTo = function() {
                var currentOffset = Math.ceil(currentYOffset);
                var len = gdToc.length;
                for (var i = 0; i < len; i++) {
                    if (currentOffset < microcache.get(gdToc[i])) return i;
                }
                return -1;
            };
            _gdFindNextOrCurrentArticleToScrollTo = function() {
                var currentOffset = Math.floor(currentYOffset);
                var len = gdToc.length;
                for (var i = 0; i < len; i++) {
                    if (currentOffset <= microcache.get(gdToc[i])) {
                        return i;
                    }
                }
                return -1;
            };
            gdMakeArticleActive = function(newId) {
                if (gdCurrentArticle !== newId) {
                    if (gdCurrentArticle) {
                        var currentActiveArticle = document.getElementById(
                            'gdarticle-' + gdCurrentArticle
                        );
                        currentActiveArticle.classList.remove(
                            'gdactivearticle'
                        );
                    }
                    var newActiveArticle = document.getElementById(
                        'gdarticle-' + newId
                    );
                    newActiveArticle.classList.add('gdactivearticle');
                    gdCurrentArticle = newId;
                    if (newId in gdAudioLinks) {
                        var activeAudio = document.getElementById(
                            'gde-activeaudio'
                        );
                        if (activeAudio) activeAudio.id = '';
                        gdAudioLink = gdAudioLinks[newId];
                        if (
                            (activeAudio = newActiveArticle.getElementsByClassName(
                                'gde-fstwav'
                            )) &&
                            activeAudio.length
                        ) {
                            activeAudio[0].id = 'gde-activeaudio';
                        }
                    }
                }
            };
            gdNextArticle = function() {
                if (gdCurrentArticle) {
                    var currentOffset = Math.floor(currentYOffset);
                    if (winH + currentOffset + 3 >= scrollHeight) {
                        if (
                            gdToc.indexOf(gdCurrentArticle) ===
                            gdToc.length - 1
                        ) {
                            if (hasScrollbar) {
                                requestAnimationFrame(function() {
                                    GDE.jump(
                                        microcache.get(gdToc[0]) -
                                            currentOffset,
                                        {
                                            duration: _duration,
                                            callback: afterScrollTimeOut
                                        }
                                    );
                                });
                            } else {
                                gdMakeArticleActive(gdToc[0]);
                            }
                        } else {
                            for (
                                var i = gdToc.indexOf(gdCurrentArticle);
                                i < gdToc.length;
                                i++
                            ) {
                                if (
                                    i !== gdToc.length - 1 &&
                                    inView(gdToc[i + 1])
                                ) {
                                    gdMakeArticleActive(gdToc[i + 1]);
                                    break;
                                }
                            }
                        }
                    } else {
                        var idx = _gdFindNextArticleToScrollTo();
                        if (idx === 0) ++idx;
                        if (idx > 0) {
                            requestAnimationFrame(function() {
                                var distance;
                                if (inView(gdToc[idx])) {
                                    distance =
                                        microcache.get(gdToc[idx]) -
                                        currentOffset;
                                } else {
                                    distance = winH - lineH;
                                }
                                if (
                                    currentOffset + distance >
                                    scrollHeight - winH
                                ) {
                                    distance =
                                        scrollHeight - winH - currentOffset;
                                    isClicked = true;
                                    GDE.jump(distance, {
                                        duration: _duration,
                                        callback: afterScrollTimeOut
                                    });
                                    gdMakeArticleActive(gdToc[idx]);
                                } else {
                                    GDE.jump(distance, {
                                        duration: _duration,
                                        callback: afterScrollTimeOut
                                    });
                                }
                            });
                        } else {
                            //gdMakeArticleActive(gdToc[gdToc.length - 1]);
                            requestAnimationFrame(function() {
                                GDE.jump(winH - lineH, {
                                    duration: _duration,
                                    callback: afterScrollTimeOut
                                });
                            });
                        }
                    }
                }
            };
            gdPreviousArticle = function() {
                if (gdCurrentArticle) {
                    var currentOffset = Math.floor(currentYOffset);
                    if (winH + currentOffset + 3 >= scrollHeight) {
                        for (
                            var i = gdToc.indexOf(gdCurrentArticle);
                            i >= 0;
                            i--
                        ) {
                            if (i !== 0) {
                                if (inView(gdToc[i - 1])) {
                                    gdMakeArticleActive(gdToc[i - 1]);
                                } else {
                                    requestAnimationFrame(function() {
                                        GDE.jump(
                                            microcache.get(gdToc[i - 1]) -
                                                currentOffset,
                                            {
                                                duration: _duration,
                                                callback: afterScrollTimeOut
                                            }
                                        );
                                    });
                                }
                                break;
                            }
                        }
                    } else {
                        var idx = _gdFindNextOrCurrentArticleToScrollTo();
                        if (idx > 0) {
                            //gdMakeArticleActive(gdToc[idx - 1]);
                            requestAnimationFrame(function() {
                                GDE.jump(
                                    microcache.get(gdToc[idx - 1]) -
                                        currentOffset,
                                    {
                                        duration: _duration,
                                        callback: afterScrollTimeOut
                                    }
                                );
                            });
                        }
                    }
                }
            };
            gdShowToc = function() {
                if (gdToc.length && gdCurrentArticle) {
                    var currentIdx = gdToc.indexOf(gdCurrentArticle);

                    if (currentIdx >= 0) {
                        var tocData = currentIdx + ' ' + gdToc.join(' ');
                        alert('showTableOfContents://' + tocData);
                    }
                }
            };
            gdJumpTo = function(id) {
                gdMakeArticleActive(id);
                requestAnimationFrame(function() {
                    GDE.jump(microcache.get(id) - Math.floor(currentYOffset), {
                        duration: _duration,
                        callback: afterScrollTimeOut
                    });
                });
            };
        };
        GDE.jump = function(target, options) {
            var start = lastYOffset || window.pageYOffset;
            var opt = {
                duration: options.duration,
                offset: options.offset || 0,
                callback: options.callback,
                easing: options.easing || easeOutQuad
            };
            var distance =
                typeof target == 'string'
                    ? opt.offset +
                      document.querySelector(target).getBoundingClientRect().top
                    : target;
            var duration =
                typeof opt.duration == 'function'
                    ? opt.duration(distance)
                    : opt.duration;
            //console.log('duration: ' + duration);
            var timeStart = null,
                timeElapsed;
            if (!isScrolling) {
                //console.log('Scroll Start!')
                window.removeEventListener('scroll', onScroll);
                document.body.classList.add('gde-scroll');
                isScrolling = true;
            }
            requestAnimationFrame(function(time) {
                timeStart = time;
                loop(time);
            });

            function loop(time) {
                timeElapsed = time - timeStart;
                window.scrollTo(
                    0,
                    opt.easing(timeElapsed, start, distance, duration)
                );
                if (timeElapsed < duration) requestAnimationFrame(loop);
                else end();
            }

            function end() {
                window.scrollTo(0, start + distance);
                typeof opt.callback == 'function' && opt.callback();
                timeStart = null;
                setTimeout(function() {
                    window.addEventListener('scroll', onScroll);
                }, 200);
            }

            function easeOutQuad(t, b, c, d) {
                return -c * (t /= d) * (t - 2) + b;
            }
        };
        return GDE;
    })();
    GDEnhancer.init();
}