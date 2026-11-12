import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Preview } from "./Preview";
const key = "a".repeat(64);
const url = `/thumbnail/example?key=${key}&scheme=light&lang=en`;
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
beforeEach(() => { vi.stubGlobal("innerWidth", 1440); });
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });
describe("image thumbnails", () => {
  it("shows a warm image without creating an artifact frame, and keeps it after focus", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({ key, url }))));
    render(<Preview id="example" version={1} />);
    await waitFor(() => expect(document.querySelector("img")).toHaveAttribute("src", url));
    const img = document.querySelector("img");
    expect(document.querySelector("iframe")).toBeNull();
    expect(img).toHaveAttribute("alt", "");
    if (!img) throw new Error("missing image");
    fireEvent.load(img);
    expect(img.closest(".thumb-screen")).toHaveClass("loaded");
    fireEvent.focus(window);
    expect(document.querySelector("img")).toBe(img);
  });
  it.each([401,403,404,503])("keeps only the placeholder on a %s response", async (status) => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(response({},status))));
    await act(async () => { render(<Preview id="example" version={1} />);  await Promise.resolve(); });
    expect(document.querySelector("img,iframe")).toBeNull();
  });
  it("ignores forged captures and accepts only its opaque frame's matching JPEG", async () => {
    let stored = false;
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === "POST") { stored = true; return Promise.resolve(new Response(null,{status:204})); }
      return Promise.resolve(response({ key, ...(stored ? {url} : {source:"/raw/example?preview&scheme=light&lang=en&viewport=desktop"}) }));
    });
    vi.stubGlobal("fetch",fetchMock);
    render(<Preview id="example" version={1} />);
    await waitFor(() => { expect(document.querySelector("iframe")).not.toBeNull(); });
    const frame = document.querySelector("iframe");
    if (!frame?.contentWindow) throw new Error("missing frame");
    expect(frame).toHaveAttribute("sandbox","allow-scripts");
    expect(frame).toHaveAttribute("aria-hidden","true");
    const sent = vi.spyOn(frame.contentWindow,"postMessage").mockImplementation(() => undefined);
    fireEvent.load(frame);
    const request = (sent.mock.calls[0]?.[0] as {request:string}).request;
    const data = {pictorPreview:"thumbnail",request,image:"data:image/jpeg;base64,YWJj"};
    await act(async () => {
      window.dispatchEvent(new MessageEvent("message",{source:window,origin:"null",data}));
      window.dispatchEvent(new MessageEvent("message",{source:frame.contentWindow,origin:location.origin,data}));
      window.dispatchEvent(new MessageEvent("message",{source:frame.contentWindow,origin:"null",data:{...data,request:"forged"}}));
     await Promise.resolve(); });
    expect(stored).toBe(false);
    await act(async () => { window.dispatchEvent(new MessageEvent("message",{source:frame.contentWindow,origin:"null",data}));  await Promise.resolve(); });
    await waitFor(() => expect(document.querySelector("img")).toHaveAttribute("src",url));
    const pendingImage=document.querySelector("img");
    if (!pendingImage) throw Error("no pending thumbnail");
    expect(document.querySelector("iframe")).toBe(frame);
    fireEvent.load(pendingImage);
    expect(document.querySelector("iframe")).toBeNull();
  });
  it("removes a failed cold capture without showing a broken image or live preview", async () => {
    vi.stubGlobal("fetch",vi.fn(() => Promise.resolve(response({key,source:"/raw/example?preview&scheme=light&lang=en&viewport=desktop"}))));
    render(<Preview id="example" version={1} />);
    await waitFor(() => { expect(document.querySelector("iframe")).not.toBeNull(); });
    const frame=document.querySelector("iframe");
    if (!frame?.contentWindow) throw new Error("missing frame");
    const sent=vi.spyOn(frame.contentWindow,"postMessage").mockImplementation(()=>undefined);fireEvent.load(frame);
    await act(async () => {window.dispatchEvent(new MessageEvent("message",{source:frame.contentWindow,origin:"null",data:{pictorPreview:"thumbnail",request:(sent.mock.calls[0]?.[0] as {request:string}).request,failed:true}})); await Promise.resolve(); });
    expect(document.querySelector("img,iframe")).toBeNull();
  });
  it("shows a cold document before image generation or upload finishes", async () => {
    vi.stubGlobal("fetch",vi.fn(()=>Promise.resolve(response({key,source:"/raw/example?preview&scheme=light&lang=en&viewport=desktop"}))));
    render(<Preview id="example" version={1} />);
    await waitFor(()=>{expect(document.querySelector("iframe")).not.toBeNull();});
    const frame=document.querySelector("iframe");if(!frame?.contentWindow)throw Error("no frame");
    vi.spyOn(frame.contentWindow,"postMessage").mockImplementation(()=>undefined);
    expect(frame.closest(".thumb-screen")).not.toHaveClass("loaded");
    fireEvent.load(frame);
    expect(frame.closest(".thumb-screen")).toHaveClass("loaded");
    expect(document.querySelector("img")).toBeNull();
    expect(frame).toHaveAttribute("data-thumbnail-viewport","desktop");
  });
  it("does not render an image from an old artifact version while its replacement loads", async () => {
    vi.stubGlobal("fetch",vi.fn(()=>Promise.resolve(response({key,url}))));
    const view=render(<Preview id="example" version={1} />);
    await waitFor(()=> { expect(document.querySelector("img")).not.toBeNull(); });
    vi.stubGlobal("fetch",vi.fn(()=>new Promise(()=>undefined)));
    view.rerender(<Preview id="example" version={2} />);
    expect(document.querySelector("img,iframe")).toBeNull();
  });
});

it("keeps the newest system-theme result when an older metadata request finishes later", async () => {
  const media=Object.assign(new EventTarget(),{matches:false});
  vi.stubGlobal("matchMedia",()=>media);
  let finish: ((value:Response)=>void) | undefined;
  let calls=0;
  const dark=`/thumbnail/example?key=${key}&scheme=dark&lang=en`;
  vi.stubGlobal("fetch",vi.fn(()=>{calls++;return calls===1?new Promise<Response>((resolve)=>{finish=resolve;}):Promise.resolve(response({key,url:dark}));}));
  render(<Preview id="example" version={1} />);
  await waitFor(()=>{expect(calls).toBe(1);});
  await act(async()=>{media.matches=true;media.dispatchEvent(new Event("change"));await Promise.resolve();});
  await waitFor(()=>{expect(document.querySelector("img")).toHaveAttribute("src",dark);});
  await act(async()=>{finish?.(response({key,url}));await Promise.resolve();});
  expect(document.querySelector("img")).toHaveAttribute("src",dark);
});

it("gives all six cold previews their full capture time after waiting for a slot", async () => {
  vi.useFakeTimers();
  const saved=new Set<string>();
  vi.stubGlobal("fetch",vi.fn((path:string,init?:RequestInit)=>{
    const id=path.split("/")[3]??"";
    if(init?.method==="POST"){saved.add(id);return Promise.resolve(new Response(null,{status:204}));}
    return Promise.resolve(response({key,...(saved.has(id)?{url:`/thumbnail/${id}?key=${key}`}:{source:`/raw/${id}?preview&scheme=light&lang=en&viewport=desktop`})}));
  }));
  await act(async()=>{render(<>{Array.from({length:6},(_,i)=><Preview key={i} id={`card-${String(i)}`} version={1}/>)}</>);await Promise.resolve();});
  const frames=[...document.querySelectorAll("iframe")];expect(frames).toHaveLength(6);
  for(const frame of frames){
    const source=frame.contentWindow;if(!source)throw Error("missing frame");
    vi.spyOn(source,"postMessage").mockImplementation((message: {request:string})=>{
      window.setTimeout(()=>{window.dispatchEvent(new MessageEvent("message",{source,origin:"null",data:{pictorPreview:"thumbnail",request:message.request,image:"data:image/jpeg;base64,YWJj"}}));},12000);
    });
  }
  await act(async()=>{frames.forEach((frame)=>{fireEvent.load(frame);});await Promise.resolve();});
  for(let pair=0;pair<3;pair++){
    await act(async()=>{await vi.advanceTimersByTimeAsync(12000);});
    await act(async()=>{document.querySelectorAll("img").forEach((image)=>{fireEvent.load(image);});await Promise.resolve();});
  }
  expect(saved.size).toBe(6);expect(document.querySelectorAll("img")).toHaveLength(6);expect(document.querySelector("iframe")).toBeNull();
});
