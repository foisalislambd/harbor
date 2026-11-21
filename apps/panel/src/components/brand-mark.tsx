type Props = {
  className?: string;
};

export function BrandMark({ className = "h-8 w-8" }: Props) {
  return (
    <img
      src="/brand/harbor-logo.png"
      alt=""
      width={128}
      height={128}
      className={`${className} shrink-0 object-contain`}
    />
  );
}
