interface SearchBoxProps {
  value: string;
  onChange: (v: string) => void;
  onSearch: () => void;
  onClear?: () => void;
  placeholder?: string;
  showClear?: boolean;
  children?: React.ReactNode;
  style?: React.CSSProperties;
}

export default function SearchBox({ value, onChange, onSearch, onClear, placeholder = 'Buscar...', showClear, children, style }: SearchBoxProps) {
  return (
    <div className="search-box" style={style}>
      <input
        placeholder={placeholder}
        value={value}
        onChange={e => onChange(e.target.value)}
        onKeyUp={e => e.key === 'Enter' && onSearch()}
      />
      {children}
      <button onClick={onSearch}>Buscar</button>
      {showClear && onClear && <button className="btn-clear" onClick={onClear}>X</button>}
    </div>
  );
}
