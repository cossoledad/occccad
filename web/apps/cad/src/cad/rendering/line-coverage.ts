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
export const HIGHLIGHT_DEPTH_VERTEX = `
attribute vec3 instanceSupportAStart,instanceSupportAEnd,instanceSupportBStart,instanceSupportBEnd;
varying float vCadHighlightDepthBias;
float cadSupportSlope(vec3 n){
  if(!(dot(n,n)>1.0e-24))return 0.0;
  float transverse=length(n.xy),z=abs(n.z);
  return z>0.25*transverse?transverse/z:4.0;
}
void cadHighlightDepth(){
  vec3 p=(modelViewMatrix*vec4(position.y<0.5?instanceStart:instanceEnd,1.0)).xyz;
  vec3 a=normalMatrix*(position.y<0.5?instanceSupportAStart:instanceSupportAEnd);
  vec3 b=normalMatrix*(position.y<0.5?instanceSupportBStart:instanceSupportBEnd);
  float slope=max(cadSupportSlope(a),cadSupportSlope(b));
  float pixelDepth=projectionMatrix[2][3]==-1.0?
    abs(projectionMatrix[3][2])/max(abs(p.z),1.0e-6):abs(projectionMatrix[2][2]);
  pixelDepth/=max(abs(projectionMatrix[1][1])*resolution.y,1.0);
  vCadHighlightDepthBias=pixelDepth*slope*(0.5*linewidth+0.5);
}
`;
