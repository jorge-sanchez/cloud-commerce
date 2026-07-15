import { Navigate, NavLink, Outlet, Route, Routes, useNavigate } from "react-router-dom";
import { clearToken, getToken } from "./api";
import Login from "./pages/Login";
import Store from "./pages/Store";
import Staff from "./pages/Staff";
import Products from "./pages/Products";
import Stock from "./pages/Stock";

function Layout() {
  const navigate = useNavigate();
  if (!getToken()) return <Navigate to="/login" replace />;

  return (
    <div className="layout">
      <nav className="sidebar">
        <h1>Cloud Commerce</h1>
        <NavLink to="/store">Store</NavLink>
        <NavLink to="/staff">Staff</NavLink>
        <NavLink to="/products">Products</NavLink>
        <NavLink to="/stock">Stock</NavLink>
        <button
          className="linklike"
          onClick={() => {
            clearToken();
            navigate("/login");
          }}
        >
          Sign out
        </button>
      </nav>
      <main>
        <Outlet />
      </main>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<Layout />}>
        <Route path="/" element={<Navigate to="/store" replace />} />
        <Route path="/store" element={<Store />} />
        <Route path="/staff" element={<Staff />} />
        <Route path="/products" element={<Products />} />
        <Route path="/stock" element={<Stock />} />
      </Route>
    </Routes>
  );
}
