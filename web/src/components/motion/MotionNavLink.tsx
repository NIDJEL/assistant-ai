import { Link, NavLink, useNavigate, type LinkProps, type NavLinkProps } from 'react-router-dom';
import { motionUpdate } from './motion';

function useMotionClick({ onClick, to, ...props }: Pick<LinkProps, 'onClick' | 'to' | 'replace' | 'state' | 'relative' | 'preventScrollReset' | 'target' | 'reloadDocument'>) {
  const navigate = useNavigate();
  const click: NonNullable<LinkProps['onClick']> = event => {
    onClick?.(event);
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || props.reloadDocument || props.target && props.target !== '_self') return;
    event.preventDefault(); motionUpdate(() => navigate(to, { replace: props.replace, state: props.state, relative: props.relative, preventScrollReset: props.preventScrollReset }));
  };
  return click;
}
export function MotionNavLink(props: NavLinkProps) {
  const onClick = useMotionClick(props);
  return <NavLink {...props} onClick={onClick}/>;
}
export function MotionLink(props: LinkProps) {
  const onClick = useMotionClick(props);
  return <Link {...props} onClick={onClick}/>;
}
