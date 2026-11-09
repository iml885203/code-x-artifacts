import { useState } from "react";
import { useT, type Key } from "../../i18n";
import type { DesignEdits } from "../edits";
import { readTree, isWithin } from "../tree";
import type { Target } from "../controller";
import { roomInline, roomPaintColor } from "./roomInline";
import { BorderControls, type BorderScope } from "./BorderControls";
import { TokenColor } from "./TokenColor";
import { useTokens } from "../tokens";
import { ColorField, Field, Section, Segmented, Toggle, type ColorChange } from "./controls";
import { formatColor, num, safeText, toLength } from "./css";
import {
  writeWrap,
  writeFillNone,
  writeFillReset,
  writeBorderSide,
  currentBorderStyle,
  type BorderPart,
  type Wrap,
  NO_SHADOW,
  readBox,
  readGrow,
  readLayout,
  readOpacity,
  readPosition,
  readShadow,
  readSize,
  readText,
  writeAlign,
  writeBoxAll,
  writeBoxPart,
  writeColor,
  writeDirection,
  writeGap,
  writeGrow,
  writeJustify,
  writeOpacity,
  writePosition,
  writeShadow,
  writeSize,
  lengthText,
  readColor,
  type Axis,
  type Box,
  type Declarations,
  type Distribution,
  type SizeMode,
} from "./model";
import "./properties.css";

interface Props {
  edits: DesignEdits;
  /** The selected elements; what is shown is the last one's, what is written goes to all of them. */
  targets: Target[];
}

const WEIGHTS = ["100", "200", "300", "400", "500", "600", "700", "800", "900"];
const DISTRIBUTION_LABELS: Record<Distribution, Key> = { start: "props.start", center: "props.center", end: "props.end", stretch: "props.stretch", between: "props.between" };


/**
 * The controls for what the selected elements look like and how they are laid
 * out. Each writes the CSS it is named for, as inline declarations in the room
 * (so everyone sees it and the saved frame carries it), and shows what the frame
 * computed for the element, whether the value came from its own style or a class.
 * Only people who can edit the newest version have it.
 */
export function PropertiesPanel({ edits, targets }: Props) {
  const t = useT();
  const tokens=useTokens(edits);
  const [refused, setRefused] = useState(false);
  const last = targets[targets.length - 1];
  if (!last) return <p className="code-note">{t("props.none")}</p>;
  const { computed, inline, rect } = last.report;

  const write = (declarations: Declarations): boolean => {
    setRefused(false);
    edits.apply(targets.map((x) => ({ file: x.file, pid: x.pid, style: declarations })));
    return true;
  };
  // A frame report arrives after the edit. Merge a channel edit with the room's
  // current declaration, so a quick alpha change cannot restore an older RGB.
  const inlineOf = (target: Target) => roomInline(edits.frame(target.file),target.pid);
  const ownStyle = (target: Target, prop: string) => inlineOf(target).get(prop)?.replace(/\s*!important\s*$/i, "");
  const colorChange = (prop: "background-color" | "border-color" | "color", change: ColorChange) => {
    setRefused(false);
    edits.apply(targets.map((target) => {
      const current = roomPaintColor(ownStyle(target, prop),tokens) ?? readColor(target.report.computed[prop === "border-color" ? "border-top-color" : prop]);
      return { file: target.file, pid: target.pid, style: writeColor(prop, change(current)) };
    }));
  };
  const shadowColorChange = (change: ColorChange) => {
    setRefused(false);
    edits.apply(targets.map((target) => {
      const current = readShadow({ "box-shadow": ownStyle(target, "box-shadow") ?? target.report.computed["box-shadow"] ?? "none" }).shadow ?? NO_SHADOW;
      return { file: target.file, pid: target.pid, style: writeShadow({ ...current, color: change(current.color) }) };
    }));
  };
  /** A length typed in a box; refused (and the box shows what it had) when it is not one. */
  const length =
    (make: (css: string) => Declarations, keywords: readonly string[] = []) =>
    (text: string): boolean => {
      const css = toLength(text, keywords);
      if (css === null) {
        setRefused(true);
        return false;
      }
      return write(make(css));
    };

  const refuse = () => {
    setRefused(true);
  };
  const layout = readLayout(computed);
  const position = readPosition(computed);
  const shadow = readShadow(computed);
  const text = readText(computed);
  const fill = computed["background-color"];
  const liveStyles = inlineOf(last);
  const liveInline: [string,string][] = [...inline.filter(([prop]) => !liveStyles.has(prop)), ...liveStyles];
  const borderComputed = {...computed};
  for (const side of ["top","right","bottom","left"] as const) {
    for (const part of ["width","style","color"]) {
      const prop = `border-${side}-${part}`;
      const value = ownStyle(last,prop);
      if (value) borderComputed[prop] = part === "color" ? formatColor(roomPaintColor(value,tokens) ?? readColor(computed[prop])) : value;
    }
  }
  for (const side of ["top","right","bottom","left"] as const) borderComputed[`border-${side}-style`] = currentBorderStyle(side,liveStyles,computed);
  const container = !["img", "input", "svg", "path", "circle", "ellipse", "rect", "line", "polygon", "polyline"].includes(last.report.tag);
  const tree = readTree(edits.map,last.file);
  const hasText = [...tree.nodes.values()].some(node => isWithin(tree,node.pid,last.pid) && (node.kind === "text" || edits.map.has(`${last.file}|n|${node.pid}|rich`)));
  const textElement = !["img", "svg", "path", "circle", "ellipse", "rect", "line", "polygon", "polyline"].includes(last.report.tag) && (hasText || ["p", "span", "h1", "h2", "h3", "h4", "h5", "h6", "a", "button", "label", "li"].includes(last.report.tag));
  const borderEdit = (scope: BorderScope, part: BorderPart, value: string | ColorChange) => {
    setRefused(false);
    edits.apply(targets.map(target => {
      const currentStyles = inlineOf(target);
      const styles: [string,string][] = [...target.report.inline.filter(([prop]) => !currentStyles.has(prop)), ...currentStyles];
      const currentComputed = {...target.report.computed, ...Object.fromEntries([...currentStyles].map(([prop,current]) => [prop,current.replace(/\s*!important\s*$/i, "")]))};
      for (const side of ["top","right","bottom","left"] as const) currentComputed[`border-${side}-style`] = currentBorderStyle(side,currentStyles,target.report.computed);
      const declarations: Declarations = {};
      for (const side of scope === "all" ? ["top", "right", "bottom", "left"] as const : [scope]) {
        const prop = `border-${side}-${part}`;
        const current = roomPaintColor(ownStyle(target,prop),tokens) ?? readColor(target.report.computed[prop]);
        Object.assign(declarations,writeBorderSide(side,part,typeof value === "string" ? value : writeColor("border-color",value(current))["border-color"] ?? "",currentComputed,styles));
      }
      return {file:target.file,pid:target.pid,style:declarations};
    }));
  };
  const fillAction = (declarations: Declarations) => {
    edits.apply(targets.map(target => ({file:target.file,pid:target.pid,style:declarations})));
  };

  const sizeRow = (axis: Axis) => {
    const size = readSize(axis, computed, inline, axis === "width" ? rect.w : rect.h);
    const label = t(axis === "width" ? "props.width" : "props.height");
    const modeLabel = t(axis === "width" ? "props.widthMode" : "props.heightMode");
    return (
      <div className="prop-row" key={axis}>
        <Field label={label} short={axis === "width" ? "W" : "H"} value={size.px} disabled={size.mode !== "fixed"} onCommit={length((css) => ({ [axis]: css }))} />
        <Segmented<SizeMode>
          label={modeLabel}
          value={size.mode}
          choices={[
            { value: "fixed", label: t("props.fixed") },
            { value: "hug", label: t("props.hug") },
            { value: "fill", label: t("props.fill") },
          ]}
          onChoose={(mode) => write(writeSize(axis, mode, Number(size.px) || (axis === "width" ? rect.w : rect.h)))}
        />
      </div>
    );
  };

  const boxRow = (box: Box, title: Key, labels: [Key, Key, Key, Key], each: Key) => (
    <BoxRow key={box} box={box} title={t(title)} labels={labels.map((l) => t(l))} eachLabel={t(each)} computed={computed} onAll={(css) => write(writeBoxAll(box, css))} onPart={(i, css, current) => write(writeBoxPart(box, i, css, current))} refuse={refuse} />
  );

  const distribution = (label: Key, value: Distribution, choices: Distribution[], make: (v: Distribution) => Declarations) => (
    <Segmented<Distribution> label={t(label)} value={value} choices={choices.map((c) => ({ value: c, label: t(DISTRIBUTION_LABELS[c]) }))} onChoose={(v) => write(make(v))} />
  );

  return (
    <div className="props-body">
      {refused && (
        <p className="error prop-refused" role="alert">
          {t("props.invalid")}
        </p>
      )}
      <Section title={t("props.size")}>
        {sizeRow("width")}
        {sizeRow("height")}
        <Toggle label={t("props.grow")} on={readGrow(computed)} onToggle={(on) => write(writeGrow(on))} />
      </Section>

      <Section title={t("props.position")}>
        <Segmented<"absolute" | "relative">
          label={t("props.position")}
          value={position === "other" ? null : position}
          choices={[
            { value: "absolute", label: t("props.absolute") },
            { value: "relative", label: t("props.relative") },
          ]}
          onChoose={(mode) => write(writePosition(mode))}
        />
        {position === "other" && <p className="code-note">{t("props.positionStatic")}</p>}
        <div className="prop-row">
          <Field label={t("props.left")} short="X" value={lengthText(computed.left)} onCommit={length((css) => ({ left: css }), ["auto"])} />
          <Field label={t("props.top")} short="Y" value={lengthText(computed.top)} onCommit={length((css) => ({ top: css }), ["auto"])} />
        </div>
      </Section>

      {container && <Section title={t("props.layout")}>
        <Segmented<"row" | "column">
          label={t("props.direction")}
          value={layout.flex ? layout.direction : null}
          choices={[
            { value: "row", label: t("props.row") },
            { value: "column", label: t("props.column") },
          ]}
          onChoose={(d) => write(writeDirection(d, computed))}
        />
        {layout.flex && (
          <>
            {distribution("props.align", layout.align, ["start", "center", "end", "stretch"], writeAlign)}
            {distribution("props.justify", layout.justify, ["start", "center", "end", "between"], writeJustify)}
            <select aria-label={t("props.wrap")} value={layout.wrap} onChange={e => write(writeWrap(e.target.value as Wrap))}>
              {!["nowrap", "wrap", "wrap-reverse"].includes(layout.wrap) && <option value={layout.wrap}>{layout.wrap}</option>}
              <option value="nowrap">{t("props.noWrap")}</option><option value="wrap">{t("props.wrap")}</option><option value="wrap-reverse">{t("props.wrapReverse")}</option>
            </select>
            <Field label={t("props.gap")} short="↔" value={layout.gap} onCommit={length(writeGap)} />
          </>
        )}
      </Section>}

      <Section title={t("props.spacing")}>
        {boxRow("padding", "props.padding", ["props.sideTop", "props.sideRight", "props.sideBottom", "props.sideLeft"], "props.eachSide")}
        {boxRow("margin", "props.margin", ["props.sideTop", "props.sideRight", "props.sideBottom", "props.sideLeft"], "props.eachSide")}
      </Section>

      <Section title={t("props.appearance")}>
        <TokenColor label={t("props.fillColor")} color={roomPaintColor(ownStyle(last,"background-color"),tokens) ?? readColor(fill)} source={liveStyles.get("background-color") ?? inline.find(([name]) => name === "background-color")?.[1]} tokens={tokens} onColor={(change) => { colorChange("background-color", change); }} onToken={(value) => write({ "background-color": value })} onNoFill={() => { fillAction(writeFillNone()); }} onReset={() => { fillAction(writeFillReset()); }} />
        <Field label={t("props.opacity")} short="%" value={readOpacity(computed)} onCommit={percent((n) => write(writeOpacity(n)))} />
        {boxRow("radius", "props.radius", ["props.cornerTL", "props.cornerTR", "props.cornerBR", "props.cornerBL"], "props.eachCorner")}
        <BorderControls computed={borderComputed} inline={liveInline} tokens={tokens} onEdit={borderEdit} onColor={(scope,change) => { borderEdit(scope,"color",change); }} onInvalid={refuse} />
        {shadow.editable ? (
          shadow.shadow ? (
            <>
              <div className="prop-row">
                <Field label={t("props.shadowX")} short="X" value={num(shadow.shadow.x)} onCommit={shadowPart(shadow.shadow, "x", write, refuse)} />
                <Field label={t("props.shadowY")} short="Y" value={num(shadow.shadow.y)} onCommit={shadowPart(shadow.shadow, "y", write, refuse)} />
                <Field label={t("props.shadowBlur")} short="B" value={num(shadow.shadow.blur)} onCommit={shadowPart(shadow.shadow, "blur", write, refuse)} />
              </div>
              <ColorField label={t("props.shadowColor")} color={shadow.shadow.color} onCommit={shadowColorChange} />
              <button type="button" onClick={() => write(writeShadow(null))}>
                {t("props.shadowOff")}
              </button>
            </>
          ) : (
            <button type="button" onClick={() => write(writeShadow(NO_SHADOW))}>
              {t("props.shadowOn")}
            </button>
          )
        ) : (
          <p className="code-note">{t("props.shadowFixed")}</p>
        )}
      </Section>

      {textElement && <Section title={t("props.text")}>
        <Field wide label={t("props.font")} value={text.family} onCommit={(v) => {
          const css = safeText(v);
          if (css === null) {
            setRefused(true);
            return false;
          }
          return write({ "font-family": css });
        }} />
        <div className="prop-row">
          <Field label={t("props.fontSize")} short="px" value={text.size} onCommit={length((css) => ({ "font-size": css }))} />
          <select aria-label={t("props.weight")} value={text.weight} onChange={(e) => write({ "font-weight": e.target.value })}>
            {(WEIGHTS.includes(text.weight) ? WEIGHTS : [...WEIGHTS, text.weight]).map((w) => (
              <option key={w} value={w}>
                {w}
              </option>
            ))}
          </select>
          <Toggle label={t("props.italic")} on={text.italic} onToggle={(on) => write({ "font-style": on ? "italic" : "normal" })} />
        </div>
        <Segmented<"left" | "center" | "right" | "justify">
          label={t("props.textAlign")}
          value={text.align}
          choices={[
            { value: "left", label: t("props.alignLeft") },
            { value: "center", label: t("props.alignCenter") },
            { value: "right", label: t("props.alignRight") },
            { value: "justify", label: t("props.alignJustify") },
          ]}
          onChoose={(a) => write({ "text-align": a })}
        />
        <div className="prop-row">
          <Field label={t("props.lineHeight")} value={text.lineHeight} onCommit={length((css) => ({ "line-height": css }), ["normal"])} />
          <Field label={t("props.letterSpacing")} value={text.letterSpacing} onCommit={length((css) => ({ "letter-spacing": css }), ["normal"])} />
        </div>
        <TokenColor label={t("props.textColor")} color={roomPaintColor(ownStyle(last,"color"),tokens) ?? readColor(computed.color)} source={ownStyle(last,"color") ?? inline.find(([name]) => name === "color")?.[1]} tokens={tokens} onColor={(change) => { colorChange("color", change); }} onToken={(value) => write({ "color": value })} />
      </Section>}
    </div>
  );

  /** A percentage typed in a box. */
  function percent(make: (n: number) => boolean) {
    return (typed: string): boolean => {
      const n = Number(typed);
      if (typed.trim() === "" || !Number.isFinite(n)) {
        setRefused(true);
        return false;
      }
      return make(n);
    };
  }
}

function shadowPart(current: NonNullable<ReturnType<typeof readShadow>["shadow"]>, part: "x" | "y" | "blur", write: (d: Declarations) => boolean, refuse: () => void) {
  return (typed: string): boolean => {
    const css = toLength(typed);
    const n = css ? Number.parseFloat(css) : NaN;
    if (!css?.endsWith("px") || Number.isNaN(n)) {
      refuse();
      return false;
    }
    return write(writeShadow({ ...current, [part]: n }));
  };
}

interface BoxRowProps {
  box: Box;
  title: string;
  labels: string[];
  eachLabel: string;
  computed: Record<string, string | undefined>;
  onAll: (css: string) => void;
  onPart: (index: number, css: string, current: string[]) => void;
  refuse: () => void;
}

/** Padding, margin or the radius: one box for all four, or four, as the person chooses (four to begin with when they differ). */
function BoxRow({ box, title, labels, eachLabel, computed, onAll, onPart, refuse }: BoxRowProps) {
  const { values, same } = readBox(box, computed);
  const [each, setEach] = useState(!same);
  const commit = (apply: (css: string) => void) => (typed: string) => {
    const css = toLength(typed);
    if (css === null) {
      refuse();
      return false;
    }
    apply(css);
    return true;
  };
  return (
    <div className="prop-box" role="group" aria-label={title}>
      <span className="prop-title">{title}</span>
      <div className="prop-row">
        {each ? (
          values.map((v, i) => <Field key={labels[i]} label={`${title}, ${labels[i] ?? ""}`} value={v} onCommit={commit((css) => { onPart(i, css, values); })} />)
        ) : (
          <Field label={title} value={same ? (values[0] ?? "") : ""} placeholder={same ? undefined : "…"} onCommit={commit(onAll)} />
        )}
        <Toggle label={eachLabel} on={each} onToggle={setEach} />
      </div>
    </div>
  );
}
