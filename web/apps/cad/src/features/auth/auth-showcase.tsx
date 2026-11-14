import { CaretRightOutlined, PauseOutlined } from "@ant-design/icons";
import { useCallback, useEffect, useRef, useState } from "react";
import { Brand } from "../../components/brand";
import { ShowcaseWheel } from "./showcase-wheel";

const examples = [
  { image: "impeller.png", name: "径流叶轮", features: "旋转 · 放样 · 圆周阵列" },
  { image: "enclosure.png", name: "过渡壳体", features: "多截面放样 · 抽壳" },
  { image: "assembly.png", name: "法兰联轴器", features: "多零件 · 装配约束" },
];
const interval = 6_000;
const wrap = (index: number) => (index + examples.length) % examples.length;

export function AuthShowcase() {
  const hero = useRef<HTMLElement>(null);
  const [example, setExample] = useState(0);
  const current = useRef(0);
  const loaded = useRef(new Set<number>());
  const pending = useRef<number | undefined>(undefined);
  const [restart, setRestart] = useState(0);
  const [paused, setPaused] = useState(() => matchMedia("(prefers-reduced-motion: reduce)").matches);
  const [focused, setFocused] = useState(false);
  const [visible, setVisible] = useState(() => !document.hidden && matchMedia("(min-width: 761px)").matches);
  const select = useCallback((index: number) => {
    const next = wrap(index);
    pending.current = next;
    if (loaded.current.has(next)) { current.current = next; setExample(next); pending.current = undefined; }
    setRestart(value => value + 1);
  }, []);
  useEffect(() => {
    const media = matchMedia("(min-width: 761px)");
    const update = () => setVisible(!document.hidden && media.matches);
    document.addEventListener("visibilitychange", update);
    media.addEventListener("change", update);
    return () => { document.removeEventListener("visibilitychange", update); media.removeEventListener("change", update); };
  }, []);
  useEffect(() => {
    if (paused || focused || !visible) return;
    const timer = setTimeout(() => select(current.current + 1), interval);
    return () => clearTimeout(timer);
  }, [example, restart, paused, focused, visible, select]);
  useEffect(() => {
    const element = hero.current;
    if (!element) return;
    const gesture = new ShowcaseWheel();
    const wheel = (event: WheelEvent) => {
      if (event.ctrlKey || event.metaKey || !event.deltaY || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
      event.preventDefault();
      const direction = gesture.step(event.deltaY, event.deltaMode, element.clientHeight, performance.now());
      if (direction) select(current.current + direction);
    };
    element.addEventListener("wheel", wheel, { passive: false });
    return () => element.removeEventListener("wheel", wheel);
  }, [select]);

  return <section ref={hero} className="auth-hero" aria-label="建模示例" aria-roledescription="轮播"
    onFocusCapture={() => setFocused(true)} onBlurCapture={event => { if (!event.currentTarget.contains(event.relatedTarget)) setFocused(false); }}>
    <Brand />
    <figure className="auth-showcase">
      {examples.map((item, index) => <img key={item.image} src={`${import.meta.env.BASE_URL}showcase/${item.image}`}
        alt={`在 occccad 中建模的${item.name}`} aria-hidden={index !== example} className={index === example ? "active" : ""}
        fetchPriority={index === 0 ? "high" : "low"} draggable={false} onLoad={() => {
          loaded.current.add(index);
          if (pending.current === index) { current.current = index; setExample(index); pending.current = undefined; }
        }} />)}
      <figcaption>
        <div><span className="showcase-kicker">MADE IN OCCCCAD</span><h2>{examples[example].name}</h2><p>{examples[example].features}</p></div>
        <div className="showcase-controls">
          <button type="button" className="showcase-playback" aria-label={paused ? "播放轮播" : "暂停轮播"} onClick={() => { setPaused(value => !value); setFocused(false); }}>
            {paused ? <CaretRightOutlined /> : <PauseOutlined />}
          </button>
          <div className="showcase-pagination" role="group" aria-label="切换示例">
            {examples.map((item, index) => <button key={item.image} type="button" aria-label={item.name}
              aria-pressed={index === example} onClick={() => select(index)}><span /></button>)}
          </div>
        </div>
      </figcaption>
    </figure>
  </section>;
}
