package pptx

// The parts every file has, the same for every deck: the theme (the kit's
// light palette, so a deck prints and edits like any Office file), one master,
// one layout with a title placeholder, and the notes master.

const presProps = xmlHeader + `<p:presentationPr ` + namespaces + `/>`

const viewProps = xmlHeader + `<p:viewPr ` + namespaces + `><p:gridSpacing cx="76200" cy="76200"/></p:viewPr>`

const tableStyles = xmlHeader + `<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`

const fontScheme = `<a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/><a:font script="Hant" typeface="` + eastAsianFont + `"/>`

const themeXML = xmlHeader + `<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="CodeXArtifacts"><a:themeElements>` +
	`<a:clrScheme name="CodeXArtifacts"><a:dk1><a:srgbClr val="` + colorText + `"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="` + colorMuted + `"/></a:dk2><a:lt2><a:srgbClr val="` + colorBackground + `"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="` + colorStar + `"/></a:accent1><a:accent2><a:srgbClr val="` + colorInfo + `"/></a:accent2>` +
	`<a:accent3><a:srgbClr val="` + colorOK + `"/></a:accent3><a:accent4><a:srgbClr val="` + colorDanger + `"/></a:accent4>` +
	`<a:accent5><a:srgbClr val="` + colorStarText + `"/></a:accent5><a:accent6><a:srgbClr val="` + colorLine + `"/></a:accent6>` +
	`<a:hlink><a:srgbClr val="` + colorInfo + `"/></a:hlink><a:folHlink><a:srgbClr val="` + colorStarText + `"/></a:folHlink></a:clrScheme>` +
	`<a:fontScheme name="CodeXArtifacts"><a:majorFont>` + fontScheme + `</a:majorFont><a:minorFont>` + fontScheme + `</a:minorFont></a:fontScheme>` +
	`<a:fmtScheme name="CodeXArtifacts"><a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
	`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
	`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst></a:fmtScheme>` +
	`</a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`

const groupProps = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>` +
	`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`

const colorMap = `bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"`

var slideMaster = xmlHeader + `<p:sldMaster ` + namespaces + `><p:cSld>` +
	`<p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + colorBackground + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>` + groupProps +
	titlePlaceholder(2, `<a:xfrm><a:off x="`+emu(padX)+`" y="`+emu(padY)+`"/><a:ext cx="`+emu(innerWidth)+`" cy="`+emu(170)+`"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`) +
	// The bar the browser draws above every title.
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Accent"/><p:cNvSpPr/><p:nvPr userDrawn="1"/></p:nvSpPr><p:spPr><a:xfrm><a:off x="` + emu(padX) + `" y="0"/><a:ext cx="` + emu(96) + `" cy="` + emu(8) + `"/></a:xfrm>` +
	`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="` + colorStar + `"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr></p:sp>` +
	`</p:spTree></p:cSld><p:clrMap ` + colorMap + `/>` +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
	`<p:txStyles><p:titleStyle><a:lvl1pPr algn="l"><a:defRPr sz="3800" b="1"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mj-lt"/><a:ea typeface="+mj-ea"/><a:cs typeface="+mj-cs"/></a:defRPr></a:lvl1pPr></p:titleStyle>` +
	`<p:bodyStyle><a:lvl1pPr><a:defRPr sz="2000"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:bodyStyle>` +
	`<p:otherStyle><a:lvl1pPr><a:defRPr sz="1800"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:otherStyle></p:txStyles></p:sldMaster>`

var slideLayout = xmlHeader + `<p:sldLayout ` + namespaces + ` type="titleOnly" preserve="1"><p:cSld name="Title Only"><p:spTree>` + groupProps +
	titlePlaceholder(2, "") + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`

// titlePlaceholder is the title shape of a master or a layout; geometry is
// its spPr content, empty to take the master's.
func titlePlaceholder(id int, geometry string) string {
	return `<p:sp><p:nvSpPr><p:cNvPr id="` + itoa(id) + `" name="Title"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>` +
		`<p:spPr>` + geometry + `</p:spPr><p:txBody><a:bodyPr anchor="b"><a:normAutofit/></a:bodyPr><a:lstStyle/><a:p><a:r><a:rPr lang="en-US"/><a:t>Title</a:t></a:r></a:p></p:txBody></p:sp>`
}

var notesMaster = xmlHeader + `<p:notesMaster ` + namespaces + `><p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill><a:effectLst/></p:bgPr></p:bg><p:spTree>` + groupProps +
	`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="685800" y="685800"/><a:ext cx="5486400" cy="3086100"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln w="12700"><a:solidFill><a:srgbClr val="` + colorLine + `"/></a:solidFill></a:ln></p:spPr></p:sp>` +
	`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" sz="quarter" idx="1"/></p:nvPr></p:nvSpPr>` +
	`<p:spPr><a:xfrm><a:off x="685800" y="4114800"/><a:ext cx="5486400" cy="4572000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>` +
	`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="en-US"/></a:p></p:txBody></p:sp>` +
	`</p:spTree></p:cSld><p:clrMap ` + colorMap + `/><p:notesStyle><a:lvl1pPr><a:defRPr sz="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:notesStyle></p:notesMaster>`

// notesSlideXML is the notes page of a slide: its picture and the notes, one
// paragraph per line.
func notesSlideXML(notes string) string {
	var paras string
	for _, line := range splitLines(notes) {
		paras += `<a:p><a:r><a:rPr lang="en-US" dirty="0"/><a:t>` + esc(line) + `</a:t></a:r></a:p>`
	}
	return xmlHeader + `<p:notes ` + namespaces + `><p:cSld><p:spTree>` + groupProps +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr/></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + paras + `</p:txBody></p:sp></p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`
}
