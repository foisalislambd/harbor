const raster = new Set(["filebase", "idrive_e2"]);

export function providerLogoSrc(id: string) {
  return `/providers/${id}.${raster.has(id) ? "png" : "svg"}`;
}

export function ProviderLogo({ id, className = "h-7 w-7" }: { id: string; className?: string }) {
  return <img src={providerLogoSrc(id)} alt="" className={`${className} object-contain`} />;
}
