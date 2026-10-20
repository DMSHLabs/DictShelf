/**
 * Copyright (C) 2026 DmShAl (Shepeta Dmitry)
 *
 * SPDX-License-Identifier: GPL-3.0-or-later
 */
/* Shared ribbon behavior for the Browse and Wildcards headword control.
 * The adapters normalize index anchors or SQL-cache rows to one chip format;
 * source loading and navigation remain callbacks at each page edge. */
(()=>{
  "use strict";
  let exactCount=200;
  const maxLabelEm=12,buffer=24;
  const reportedProblems=new Set();
  const reportProblem=(onProblem,mode,reason,details={})=>{
    const key=mode+":"+reason;
    if(reportedProblems.has(key))return;
    reportedProblems.add(key);
    try{onProblem?.({mode,reason,...details})}catch(error){console.error("headword ribbon diagnostic failed",error)}
  };
  const lower=(offsets,widths,value,byEnd=false)=>{
    let lo=0,hi=Math.min(offsets.length,widths.length);
    while(lo<hi){const mid=(lo+hi)>>>1,edge=offsets[mid]+(byEnd?widths[mid]:0);if(edge<value)lo=mid+1;else hi=mid}
    return lo;
  };
  const thumbLayout=(trackWidth,visible,total,first,minPixels=20)=>{
    const count=Math.max(0,total),shown=Math.max(0,Math.min(count,visible)),maxFirst=Math.max(0,count-shown);
    const width=Math.min(trackWidth,Math.max(Math.min(minPixels,trackWidth),trackWidth*(count?shown/count:1)));
    const fraction=maxFirst?Math.max(0,Math.min(1,first/maxFirst)):0;
    return {width,left:Math.max(0,trackWidth-width)*fraction,maxFirst};
  };
  const measure=(items,sample,viewport,{gap=0,minWidth=0,fixedWidth=0,separatorWidth=0,maxWidthEm=maxLabelEm,getLabel=null,widthCache=null}={})=>{
    const style=getComputedStyle(sample),font=[style.fontStyle,style.fontVariant,style.fontWeight,style.fontSize,style.fontFamily].join(" ");
    const px=Number.parseFloat(style.fontSize)||16,pad=(Number.parseFloat(style.paddingLeft)||0)+(Number.parseFloat(style.paddingRight)||0)+(Number.parseFloat(style.borderLeftWidth)||0)+(Number.parseFloat(style.borderRightWidth)||0);
    const floor=Math.max(0,minWidth||Number.parseFloat(style.minWidth)||0),cap=px*maxWidthEm;
    const canvas=measure.canvas||(measure.canvas=document.createElement("canvas")),ctx=canvas.getContext("2d");ctx.font=font;
    const spacing=Number.parseFloat(style.letterSpacing)||0,widths=new Array(items.length),offsets=new Array(items.length);
    const n=Math.min(items.length,exactCount);let sum=0,averageCount=0,total=0;
    const widthOf=(item,index)=>{
      if(item.kind==="separator")return separatorWidth||Math.max(floor,ctx.measureText(item.label||"|").width+pad);
      if(fixedWidth)return fixedWidth;
      const label=String(getLabel?getLabel(item,index):item.label||""),key=item.kind+"\0"+label;
      const cached=widthCache?.get(key);if(cached!==undefined)return cached;
      const text=ctx.measureText(label).width+Math.max(0,label.length-1)*spacing;
      const width=Math.max(floor,Math.min(cap,text+pad));widthCache?.set(key,width);return width;
    };
    for(let i=0;i<n;i++){const w=widthOf(items[i],i);widths[i]=w;if(items[i].kind!=="separator"){sum+=w;averageCount++}}
    const average=averageCount?sum/averageCount:Math.max(floor,pad),limit=Math.max(n,items.length);
    for(let i=0;i<limit;i++){
      if(i>=n)widths[i]=items[i].kind==="separator"?widthOf(items[i],i):fixedWidth||average;
      offsets[i]=total;total+=widths[i];if(i+1<items.length)total+=gap;
    }
    return {widths,offsets,total:Math.max(total,viewport.clientWidth),viewport:viewport.clientWidth};
  };
  const paint=(track,items,geometry,left,activeKey,from,to,{buttonClass="ribbon-chip",pendingClass="ribbon-pending",separatorClass="ribbon-separator",sampleNode=null,contentWidth=false,bands=[],onClick,decorate,adapt,mode="ribbon",onProblem}={})=>{
    const offsets=geometry?.offsets||[],widths=geometry?.widths||[],length=Math.min(items?.length||0,offsets.length,widths.length);
    if(length!==items?.length||offsets.length!==widths.length||!Number.isInteger(from)||!Number.isInteger(to)||from<0||to>length||from>to)
      reportProblem(onProblem,mode,"geometry",{items:items?.length||0,offsets:offsets.length,widths:widths.length,from,to});
    const begin=Math.max(0,Math.min(length,Number.isInteger(from)?from:0));
    const end=Math.max(begin,Math.min(length,Number.isInteger(to)?to:begin));
    const fragment=document.createDocumentFragment();
    const visibleStart=begin<end?offsets[begin]:0,visibleEnd=begin<end?offsets[end-1]+widths[end-1]:0;
    for(const band of bands){
      if(!Number.isFinite(band.start)||!Number.isFinite(band.end)||band.end<=visibleStart||band.start>=visibleEnd)continue;
      const el=document.createElement("span");el.className="mask-dictionary-band";el.dataset.tone=String(Math.max(0,Math.floor(Number(band.tone)||0))%4);
      el.dataset.ribbonStart=String(band.start);el.style.left=(band.start-left)+"px";el.style.width=(band.end-band.start)+"px";fragment.appendChild(el);
    }
    for(let i=begin;i<end;i++){
      const item=adapt?adapt(items[i],i):items[i];
      if(!item||!Number.isFinite(offsets[i])||!Number.isFinite(widths[i])||widths[i]<0){
        reportProblem(onProblem,mode,"entry",{items:items.length,offsets:offsets.length,widths:widths.length,from:i,to:i+1});
        continue;
      }
      const el=document.createElement(item.kind==="chip"?"button":"span");
      if(item.kind==="chip"){
        el.type="button";el.className="wudict-ribbon-chip"+(buttonClass?" "+buttonClass:"");
        const target=item.target||item;
        if(target.dict!==undefined)el.dataset.dict=target.dict;
        if(Number.isInteger(item.groupIndex))el.dataset.dictionaryGroup=String(item.groupIndex%4);
        if(target.page!==undefined)el.dataset.page=String(target.page);
        if(target.offset!==undefined)el.dataset.o=String(target.offset);
        el.setAttribute("aria-current",String(item.key===activeKey));
        if(onClick)el.addEventListener("click",()=>onClick(item));
      }else{el.className=item.kind==="separator"?separatorClass:pendingClass;if(Number.isInteger(item.groupIndex))el.dataset.dictionaryGroup=String(item.groupIndex%4)}
      el.textContent=String(item.label||"");el.title=item.title||"";
      if(item.kind==="chip")el.setAttribute("aria-label",String(item.label||""));
      // Browse uses native scrollLeft; Wildcards moves virtual chips itself.
      el.style.left=(offsets[i]-(contentWidth?0:left))+"px";el.style.width=widths[i]+"px";el.dataset.stripIndex=String(i);
      if(decorate)decorate(el,item,i);
      fragment.appendChild(el);
    }
    if(sampleNode)fragment.appendChild(sampleNode);
    track.replaceChildren(fragment);
    const measuredTotal=length?offsets[length-1]+widths[length-1]:0;
    track.style.width=contentWidth?Math.max(0,Number(geometry?.viewport)||0,Number(geometry?.total)||0,Number.isFinite(measuredTotal)?measuredTotal:0)+"px":"100%";
    track.style.minWidth=contentWidth?track.style.width:"100%";
  };
  // Both views feed this same control with a normalized list. The source
  // adapters deliberately stop at data conversion: Browse reads the prepared
  // dictionary index, while Wildcards owns the SQL cache and its page loader.
  const fromBrowseIndex=(letters,{labelOf=item=>item?.l,title=()=>""}={})=>letters.map((item,index)=>{
    const label=String(labelOf(item,index)??"");
    return {kind:"chip",key:String(index),label,title:title(item,index,label),target:{offset:Number(item?.o)}};
  });
  const fromWildcardSQL=(rows,{allWords=false,pageSize=300,reservedBlocks=()=>1,title=(_row,_item,label)=>label}={})=>{
    const groups=[];
    for(const row of rows){
      const groupIndex=groups.length;
      const complete=row.state==="ready"&&(!allWords||!row.partial),entries=[];
      if(complete){
        if(!row.total)continue;
        for(const letter of row.alphabet||[]){
          const page=Math.floor(letter.o/(row.size||pageSize))+1,label=String(letter.l||"");
          entries.push({kind:"chip",key:row.id+":"+page,label,dict:row.id,page,groupIndex,
            target:{dict:row.id,page},title:title(row,letter,label)});
        }
      }else{
        const count=Math.max(0,Number(reservedBlocks(row))||0);
        for(let page=1;page<=count;page++)entries.push({kind:"pending",label:row.state==="error"?"⚠":page===1?"…":"",
          dict:row.id,page,groupIndex,target:{dict:row.id,page},title:row.name||""});
      }
      if(entries.length)groups.push(entries);
    }
    return groups.flatMap((entries,index)=>index?[{kind:"separator",label:"|"},...entries]:entries);
  };
  function createScroller({viewport,track,bar,thumb,previous=null,next=null,getItems,getOffsets,getWidths,getPosition,setPosition,getMaxScroll,
    isSelectable=item=>item?.kind!=="separator",thumbPosition="left",onMove,onFinish,onRender,onCancel,minRenderInterval=0}={}){
    if(!viewport||!track||!bar||!thumb||typeof getItems!=="function"||typeof getOffsets!=="function"||
       typeof getWidths!=="function"||typeof getPosition!=="function"||typeof setPosition!=="function"||typeof getMaxScroll!=="function")
      throw new TypeError("headword ribbon scroller needs a viewport, scrollbar, and position adapters");
    let drag=null,touch=null,paintTimer=0,lastPaint=0,cachedItems=null,cachedLength=-1,selectableIndexes=[];
    const cancelGesture=()=>{
      const pointerId=drag?.pointerId,active=!!(drag||touch);
      drag=null;touch=null;
      if(pointerId!==undefined&&bar.hasPointerCapture?.(pointerId))bar.releasePointerCapture(pointerId);
      if(active)onCancel?.();
    };
    const rebuildSelectable=()=>{
      const items=getItems()||[];if(items===cachedItems&&items.length===cachedLength)return items;
      if(cachedItems!==null)cancelGesture();
      cachedItems=items;cachedLength=items.length;selectableIndexes=[];
      for(let i=0;i<items.length;i++)if(isSelectable(items[i]))selectableIndexes.push(i);
      return items;
    };
    const sectionAt=(position,byEnd=false)=>{
      const offsets=getOffsets()||[],widths=getWidths()||[];let lo=0,hi=selectableIndexes.length;
      while(lo<hi){const mid=(lo+hi)>>>1,index=selectableIndexes[mid],edge=(offsets[index]||0)+(byEnd?(widths[index]||0):0);if(edge<position)lo=mid+1;else hi=mid}
      return lo;
    };
    const offsetAt=section=>{const raw=selectableIndexes[section];return raw===undefined?0:(getOffsets()||[])[raw]||0};
    const widthAt=section=>{const raw=selectableIndexes[section];return raw===undefined?0:(getWidths()||[])[raw]||0};
    const clamp=value=>Math.max(0,Math.min(Math.max(0,getMaxScroll()),Number(value)||0));
    const update=()=>{
      const items=rebuildSelectable(),position=clamp(getPosition()),right=position+viewport.clientWidth;
      const first=sectionAt(position,true),end=sectionAt(right),visible=Math.max(0,end-first),layout=thumbLayout(bar.clientWidth,visible,selectableIndexes.length,first);
      thumb.style.width=layout.width+"px";
      if(thumbPosition==="transform"){thumb.style.left="0px";thumb.style.transform=`translateX(${layout.left}px)`}
      else{thumb.style.transform="";thumb.style.left=layout.left+"px"}
      bar.setAttribute("aria-valuemax",String(Math.max(0,selectableIndexes.length-1)));
      bar.setAttribute("aria-valuenow",String(Math.min(Math.max(0,selectableIndexes.length-1),first)));
      bar.setAttribute("aria-disabled",String(selectableIndexes.length<=visible));
      if(previous)previous.disabled=position<=1;
      if(next)next.disabled=position>=Math.max(0,getMaxScroll())-1;
      return {items,first,end,visible,position};
    };
    const paintSoon=()=>{
      if(typeof onRender!=="function")return;
      if(paintTimer)return;
      const wait=Math.max(0,minRenderInterval-(performance.now()-lastPaint));
      paintTimer=setTimeout(()=>{paintTimer=0;requestAnimationFrame(()=>{lastPaint=performance.now();onRender()})},wait);
    };
    const refresh=()=>{update();paintSoon()};
    const setAtThumb=(clientX)=>{
      const state=update(),rect=bar.getBoundingClientRect(),maxThumb=Math.max(0,bar.clientWidth-thumb.offsetWidth);
      const thumbLeft=Math.max(0,Math.min(maxThumb,clientX-rect.left-(drag?.grab||0)));
      const first=Math.round((maxThumb?thumbLeft/maxThumb:0)*Math.max(0,selectableIndexes.length-Math.max(1,state.visible)));
      const raw=selectableIndexes[Math.min(first,Math.max(0,selectableIndexes.length-1))];
      const position=thumbLeft>=maxThumb?getMaxScroll():raw===undefined?0:(getOffsets()||[])[raw];
      setPosition(clamp(position));
      const moved=update(),section=sectionAt(moved.position,true),index=selectableIndexes[Math.min(section,Math.max(0,selectableIndexes.length-1))];
      onMove?.({section,index,item:index===undefined?null:moved.items[index]});
      paintSoon();
    };
    const onBarDown=event=>{
      if(event.button!==0||event.isPrimary===false)return;
      update();
      const rect=thumb.getBoundingClientRect(),onThumb=event.target===thumb;
      drag={pointerId:event.pointerId,grab:onThumb?event.clientX-rect.left:rect.width/2,onThumb,moved:false,items:cachedItems,length:cachedLength};
      if(!onThumb)setAtThumb(event.clientX);
      bar.setPointerCapture?.(event.pointerId);event.preventDefault();
    };
    const onBarMove=event=>{
      if(!drag||event.pointerId!==drag.pointerId)return;
      drag.moved=true;setAtThumb(event.clientX);event.preventDefault();
    };
    const onBarEnd=event=>{
      if(!drag||event?.pointerId!==undefined&&event.pointerId!==drag.pointerId)return;
      const items=getItems()||[];
      if(event.type!=="pointerup"||items!==drag.items||items.length!==drag.length){cancelGesture();refresh();return}
      const shouldSelect=!drag.onThumb||drag.moved;drag=null;
      if(shouldSelect){const state=update(),center=state.position+viewport.clientWidth/2;
        const first=sectionAt(state.position,true),last=Math.max(0,sectionAt(state.position+viewport.clientWidth)-1);
        let section=Math.min(selectableIndexes.length-1,sectionAt(center));
        if(state.position<=1)section=0;
        else if(state.position>=Math.max(0,getMaxScroll())-1)section=selectableIndexes.length-1;
        else if(section>first&&section<selectableIndexes.length&&Math.abs(offsetAt(section-1)+widthAt(section-1)/2-center)<Math.abs(offsetAt(section)+widthAt(section)/2-center))section--;
        section=Math.max(first,Math.min(last,section));
        const index=selectableIndexes[section];onFinish?.({section,index,item:index===undefined?null:state.items[index]});
      }
      refresh();
    };
    const onKey=event=>{
      const step=Math.max(40,viewport.clientWidth/5);let position=Number(getPosition())||0;
      if(event.key==="ArrowLeft")position-=step;else if(event.key==="ArrowRight")position+=step;
      else if(event.key==="PageUp")position-=viewport.clientWidth;else if(event.key==="PageDown")position+=viewport.clientWidth;
      else if(event.key==="Home")position=0;else if(event.key==="End")position=getMaxScroll();else return;
      setPosition(clamp(position));refresh();event.preventDefault();
    };
    const onWheel=event=>{
      const delta=Math.abs(event.deltaX)>Math.abs(event.deltaY)?event.deltaX:event.shiftKey?event.deltaY:0;
      if(!delta||getMaxScroll()<=0)return;
      setPosition(clamp((Number(getPosition())||0)+delta));refresh();event.preventDefault();
    };
    const onTouchDown=event=>{if(event.pointerType==="touch"&&event.button===0)touch={id:event.pointerId,startX:event.clientX,startPosition:Number(getPosition())||0,dragging:false}};
    const onTouchMove=event=>{
      if(!touch||event.pointerId!==touch.id)return;
      const delta=touch.startX-event.clientX;
      if(!touch.dragging&&Math.abs(delta)<6)return;
      touch.dragging=true;setPosition(clamp(touch.startPosition+delta));refresh();event.preventDefault();
    };
    const onTouchEnd=event=>{if(!touch||event?.pointerId!==undefined&&event.pointerId!==touch.id)return;touch=null};
    const scrollBy=(direction,behavior="auto")=>{
      const distance=Math.max(80,viewport.clientWidth*.75),position=clamp((Number(getPosition())||0)+direction*distance);
      setPosition(position,behavior);refresh();
    };
    const onPrevious=()=>scrollBy(-1,"smooth"),onNext=()=>scrollBy(1,"smooth");
    bar.addEventListener("pointerdown",onBarDown);bar.addEventListener("pointermove",onBarMove);
    for(const type of ["pointerup","pointercancel","lostpointercapture"])bar.addEventListener(type,onBarEnd);
    bar.addEventListener("keydown",onKey);viewport.addEventListener("wheel",onWheel,{passive:false});
    viewport.addEventListener("pointerdown",onTouchDown,{passive:true});
    window.addEventListener("pointermove",onTouchMove,{capture:true,passive:false});
    for(const type of ["pointerup","pointercancel","blur"])window.addEventListener(type,onTouchEnd,true);
    viewport.addEventListener("scroll",refresh,{passive:true});
    previous?.addEventListener("click",onPrevious);next?.addEventListener("click",onNext);
    return Object.freeze({update,refresh,scrollBy,invalidate(){cancelGesture();cachedItems=null;cachedLength=-1;refresh()},get dragging(){return !!(drag||touch?.dragging)},destroy(){
      cancelGesture();
      if(paintTimer)clearTimeout(paintTimer);
      bar.removeEventListener("pointerdown",onBarDown);bar.removeEventListener("pointermove",onBarMove);
      for(const type of ["pointerup","pointercancel","lostpointercapture"])bar.removeEventListener(type,onBarEnd);
      bar.removeEventListener("keydown",onKey);viewport.removeEventListener("wheel",onWheel);viewport.removeEventListener("pointerdown",onTouchDown);
      window.removeEventListener("pointermove",onTouchMove,true);
      for(const type of ["pointerup","pointercancel","blur"])window.removeEventListener(type,onTouchEnd,true);
      viewport.removeEventListener("scroll",refresh);previous?.removeEventListener("click",onPrevious);next?.removeEventListener("click",onNext);
    }});
  }
  const api={buffer,lower,measure,paint,thumbLayout,fromBrowseIndex,fromWildcardSQL,createScroller,
    setExactCount:value=>{const n=Number(value);if(Number.isInteger(n)&&n>0)exactCount=n}};
  // One public control surface owns both the search/mask dock and ribbon.
  window.wudictHeadwordControl?.registerRibbon(api);
})();
