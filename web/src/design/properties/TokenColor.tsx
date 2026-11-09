import { ColorPicker, type ColorPickerProps } from "./ColorPicker";

/** The computed swatch previews the selected source; a token choice writes its binding. */
export function TokenColor(props: ColorPickerProps) {
  return <div className="prop-paint"><span className="prop-title">{props.label}</span><ColorPicker {...props} /></div>;
}
