/** Keep Three's analytic round caps, with coverage applied exactly once by
 * blending. Hardware alpha-to-coverage plus transparent blending attenuates
 * short curved segments twice and quantizes them into visible pale joints. */
export function analyticLineCoverage(fragment: string): string {
  return "#define CAD_ANALYTIC_LINE_COVERAGE\n" + fragment
    .replaceAll("USE_ALPHA_TO_COVERAGE", "CAD_ANALYTIC_LINE_COVERAGE")
    .replace("float alpha = opacity;", "float alpha = 1.0;")
    .replace("#include <tonemapping_fragment>", "gl_FragColor.a *= opacity;\n#include <tonemapping_fragment>");
}

/** Raster tolerance for the stroke footprint on its own support surface.
 * One view-space pixel, converted to window depth, times the support slope
 * and stroke radius. Clamp the slope to 4: grazing normals cannot grant an
 * unbounded depth offset. Inverse-transpose normalMatrix handles instances. */
export const LINE_SUPPORT_DEPTH_VERTEX = `
varying float vCadLineSupportDepthBias;
varying float vCadLinePixelDepth;
float cadLinePixelDepth(float clipW){
  float depth=projectionMatrix[2][3]==-1.0?
    abs(projectionMatrix[3][2])/max(abs(clipW),1.0e-6):abs(projectionMatrix[2][2]);
  return depth/max(abs(projectionMatrix[1][1])*resolution.y,1.0);
}
float cadSupportSlope(vec3 n){
  if(!(dot(n,n)>1.0e-24))return 0.0;
  float transverse=length(n.xy),z=abs(n.z);
  return z>0.25*transverse?transverse/z:4.0;
}
float cadLineSupportDepth(vec3 a,vec3 b){
  vec3 p=(modelViewMatrix*vec4(position.y<0.5?instanceStart:instanceEnd,1.0)).xyz;
  float slope=max(cadSupportSlope(a),cadSupportSlope(b));
  return cadLinePixelDepth(p.z)*slope*(0.5*linewidth+0.5);
}
`;

export const HIGHLIGHT_DEPTH_VERTEX = LINE_SUPPORT_DEPTH_VERTEX + `
attribute vec3 instanceSupportAStart,instanceSupportAEnd,instanceSupportBStart,instanceSupportBEnd;
void cadHighlightDepth(){
  vec3 a=normalMatrix*(position.y<0.5?instanceSupportAStart:instanceSupportAEnd);
  vec3 b=normalMatrix*(position.y<0.5?instanceSupportBStart:instanceSupportBEnd);
  vCadLineSupportDepthBias=cadLineSupportDepth(a,b);
}
`;
