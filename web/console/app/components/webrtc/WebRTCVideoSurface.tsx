import type { RefObject } from "react";
import { assignStreamToVideoElement, type normalizeDisplayLayout } from "./webrtcMediaLayout";

type WebRTCVideoSurfaceProps = {
  connected: boolean;
  scalingMode: "native" | "fit" | "fill";
  stitchedLayout: ReturnType<typeof normalizeDisplayLayout>;
  videoRef: RefObject<HTMLVideoElement | null>;
  stitchedStageRef: RefObject<HTMLDivElement | null>;
  stitchedVideoRefs: RefObject<Array<HTMLVideoElement | null>>;
  streamRef: RefObject<MediaStream | null>;
};

export function WebRTCVideoSurface({ connected, scalingMode, stitchedLayout,
  videoRef, stitchedStageRef, stitchedVideoRefs, streamRef,
}: WebRTCVideoSurfaceProps) {
  const nativeScaling = scalingMode === "native";
  const videoStyle: React.CSSProperties = nativeScaling
    ? {
        width: "auto",
        height: "auto",
        maxWidth: "none",
        maxHeight: "none",
        objectFit: "none",
        display: "block",
        cursor: connected ? "none" : "default",
      }
    : {
        width: "100%",
        height: "100%",
        objectFit: scalingMode === "fill" ? "cover" : "contain",
        display: "block",
        cursor: connected ? "none" : "default",
      };
  const stitchedStageStyle: React.CSSProperties | undefined =
    stitchedLayout
      ? {
          position: "relative",
          aspectRatio: nativeScaling
            ? undefined
            : `${stitchedLayout.totalWidth} / ${stitchedLayout.totalHeight}`,
          width: nativeScaling ? stitchedLayout.totalWidth : "100%",
          maxWidth: nativeScaling ? "none" : "100%",
          maxHeight: nativeScaling ? "none" : "100%",
          height: nativeScaling
            ? stitchedLayout.totalHeight
            : scalingMode === "fill"
              ? "100%"
              : "auto",
          overflow: "hidden",
          background: "var(--panel)",
        }
      : undefined;

  return (
    <>
        {stitchedLayout ? (
          <>
            <video
              ref={videoRef}
              style={{ display: "none" }}
              autoPlay
              playsInline
            />
            <div
              ref={stitchedStageRef}
              data-webrtc-layout="stitched"
              style={stitchedStageStyle}
            >
              {stitchedLayout.displays.map((display, index) => (
                <div
                  key={display.name}
                  data-webrtc-display={display.name}
                  style={{
                    position: "absolute",
                    left: `${display.leftPct}%`,
                    top: `${display.topPct}%`,
                    width: `${display.widthPct}%`,
                    height: `${display.heightPct}%`,
                    overflow: "hidden",
                    border: "1px solid rgba(255,255,255,0.08)",
                    borderRadius: "12px",
                    boxShadow: "0 10px 24px rgba(0,0,0,0.22)",
                  }}
                >
                  <video
                    ref={(node) => {
                      stitchedVideoRefs.current[index] = node;
                      assignStreamToVideoElement(node, streamRef.current, false, 0);
                    }}
                    style={{
                      position: "absolute",
                      left: `${-((display.leftPct / Math.max(display.widthPct, 0.01)) * 100)}%`,
                      top: `${-((display.topPct / Math.max(display.heightPct, 0.01)) * 100)}%`,
                      width: `${(100 / Math.max(display.widthPct, 0.01)) * 100}%`,
                      height: `${(100 / Math.max(display.heightPct, 0.01)) * 100}%`,
                      objectFit: "fill",
                      cursor: connected ? "none" : "default",
                    }}
                    autoPlay
                    playsInline
                  />
                  <div
                    style={{
                      position: "absolute",
                      left: 10,
                      top: 10,
                      padding: "4px 8px",
                      borderRadius: 999,
                      background: "rgba(7, 10, 18, 0.72)",
                      color: "rgba(255,255,255,0.92)",
                      fontSize: 12,
                      fontWeight: 600,
                      letterSpacing: 0.2,
                    }}
                  >
                    {display.name}
                  </div>
                </div>
              ))}
            </div>
          </>
        ) : (
          <video ref={videoRef} style={videoStyle} autoPlay playsInline />
        )}
    </>
  );
}
